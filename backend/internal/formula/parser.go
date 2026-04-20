// Package formula implements a safe expression parser that converts user-written
// formulas into PostgreSQL expressions.
//
// The parser follows a three-stage pipeline:
//  1. Lexer (lex): tokenizes the input string into numbers, strings, identifiers, operators
//  2. Parser: validates syntax, checks field slug references, resolves functions
//  3. SQL generator: emits a parameterized PostgreSQL expression
//
// Only whitelisted functions are allowed (SUM, AVG, IF, COALESCE, etc.) to prevent
// SQL injection. Field references are validated against the collection's field slugs.
// Cross-collection functions (LOOKUP, SUMREL, etc.) use a RelationResolver callback
// to generate safe subqueries.
package formula

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/choiceoh/phaeton/backend/internal/pgutil"
)

// tokenType classifies lexer tokens.
type tokenType int

const (
	tokNumber tokenType = iota
	tokString
	tokIdent
	tokOperator
	tokComma
	tokLParen
	tokRParen
	tokDot
	tokEOF
)

type token struct {
	typ tokenType
	val string
}

// Supported aggregate/scalar functions (whitelist).
var allowedFunctions = map[string]bool{
	"SUM": true, "AVG": true, "MIN": true, "MAX": true, "COUNT": true,
	"ROUND": true, "CEIL": true, "FLOOR": true, "ABS": true,
	"COALESCE": true, "NULLIF": true,
	"IF": true,
}

// Cross-collection functions.
var crossCollectionFunctions = map[string]bool{
	"LOOKUP":   true,
	"SUMREL":   true,
	"AVGREL":   true,
	"MINREL":   true,
	"MAXREL":   true,
	"COUNTREL": true,
}

// RelationInfo describes a resolved relation for cross-collection formulas.
type RelationInfo struct {
	// TargetTable is the fully qualified target table (e.g. "data"."customers").
	TargetTable string
	// OwnerColumn is the local FK column name (for LOOKUP).
	OwnerColumn string
	// ReverseColumn is the column on the target table pointing back (for *REL aggregates).
	ReverseColumn string
}

// RelationResolver is called by the parser when it encounters a cross-collection
// function. It receives the relation field slug and should return the resolved
// relation metadata, or an error if the field is not a valid relation.
type RelationResolver func(relationSlug string) (*RelationInfo, error)

// lex tokenizes a formula expression string into a slice of tokens.
// Recognition rules:
//   - Numbers: digit sequences with optional decimal point (e.g. "3.14")
//   - Identifiers: letter/underscore start, then letters/digits/underscores (field slugs or function names)
//   - Strings: single-quoted with backslash escaping (e.g. 'hello')
//   - Operators: two-char (<=, >=, !=, <>) and single-char (+, -, *, /, %, <, >, =)
//   - Delimiters: comma, parentheses, dot
//
// Returns an error for unterminated strings or unexpected characters.
func lex(input string) ([]token, error) {
	var tokens []token
	i := 0
	for i < len(input) {
		ch := rune(input[i])

		// Skip whitespace.
		if unicode.IsSpace(ch) {
			i++
			continue
		}

		// Numbers (including decimals).
		if unicode.IsDigit(ch) || (ch == '.' && i+1 < len(input) && unicode.IsDigit(rune(input[i+1]))) {
			start := i
			for i < len(input) && (unicode.IsDigit(rune(input[i])) || input[i] == '.') {
				i++
			}
			tokens = append(tokens, token{tokNumber, input[start:i]})
			continue
		}

		// Identifiers (field slugs, function names).
		if unicode.IsLetter(ch) || ch == '_' {
			start := i
			for i < len(input) && (unicode.IsLetter(rune(input[i])) || unicode.IsDigit(rune(input[i])) || input[i] == '_') {
				i++
			}
			tokens = append(tokens, token{tokIdent, input[start:i]})
			continue
		}

		// String literals (single-quoted).
		if ch == '\'' {
			i++
			start := i
			for i < len(input) && input[i] != '\'' {
				if input[i] == '\\' {
					i++ // skip escaped char
				}
				i++
			}
			if i >= len(input) {
				return nil, fmt.Errorf("unterminated string literal")
			}
			tokens = append(tokens, token{tokString, input[start:i]})
			i++ // skip closing quote
			continue
		}

		// Two-character operators.
		if i+1 < len(input) {
			two := input[i : i+2]
			if two == "<=" || two == ">=" || two == "!=" || two == "<>" {
				tokens = append(tokens, token{tokOperator, two})
				i += 2
				continue
			}
		}

		// Single-character operators and delimiters.
		switch ch {
		case '+', '-', '*', '/', '%', '<', '>', '=':
			tokens = append(tokens, token{tokOperator, string(ch)})
			i++
		case ',':
			tokens = append(tokens, token{tokComma, ","})
			i++
		case '(':
			tokens = append(tokens, token{tokLParen, "("})
			i++
		case ')':
			tokens = append(tokens, token{tokRParen, ")"})
			i++
		case '.':
			tokens = append(tokens, token{tokDot, "."})
			i++
		default:
			return nil, fmt.Errorf("unexpected character %q at position %d", ch, i)
		}
	}
	tokens = append(tokens, token{tokEOF, ""})
	return tokens, nil
}

// Parser converts a token stream into a safe PostgreSQL expression.
// It maintains parsing state:
//   - tokens/pos: the token stream and current position (recursive descent)
//   - slugs: set of valid field slugs for the current collection (for validation)
//   - refSlugs: accumulates local field slugs referenced in the expression (output)
//   - crossRefs: accumulates relation field slugs used in cross-collection functions (output)
//   - resolver: optional callback to resolve relation metadata for LOOKUP/SUMREL/etc.
type Parser struct {
	tokens    []token
	pos       int
	slugs     map[string]bool // valid field slugs for this collection
	refSlugs  []string        // referenced field slugs (output)
	crossRefs []string        // cross-collection relation slugs referenced
	resolver  RelationResolver
}

func (p *Parser) peek() token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return token{tokEOF, ""}
}

func (p *Parser) advance() token {
	t := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *Parser) expect(typ tokenType) (token, error) {
	t := p.advance()
	if t.typ != typ {
		return t, fmt.Errorf("expected %d, got %q", typ, t.val)
	}
	return t, nil
}

// ParseResult contains the output of a successful parse.
type ParseResult struct {
	SQL       string   // Safe PostgreSQL expression.
	Refs      []string // Local field slugs referenced.
	CrossRefs []string // Relation field slugs used in cross-collection functions.
}

// Parse parses a formula expression string and returns a safe PostgreSQL SQL expression
// along with the list of local field slugs referenced. This is the simple entry point
// that does not support cross-collection functions (LOOKUP, SUMREL, etc.).
// Use ParseWithResolver for cross-collection support.
func Parse(expression string, validSlugs map[string]bool) (sql string, refs []string, err error) {
	result, err := ParseWithResolver(expression, validSlugs, nil)
	if err != nil {
		return "", nil, err
	}
	return result.SQL, result.Refs, nil
}

// ParseWithResolver parses a formula expression with support for cross-collection
// functions (LOOKUP, SUMREL, etc.) via the provided resolver callback.
// It strips a leading '=' (Excel convention), tokenizes, runs the recursive descent
// parser, and returns a ParseResult containing the SQL, local refs, and cross-refs.
func ParseWithResolver(expression string, validSlugs map[string]bool, resolver RelationResolver) (*ParseResult, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, fmt.Errorf("empty expression")
	}
	// Strip leading '=' if present (Excel convention).
	if expression[0] == '=' {
		expression = expression[1:]
	}

	tokens, err := lex(expression)
	if err != nil {
		return nil, err
	}

	p := &Parser{tokens: tokens, slugs: validSlugs, resolver: resolver}
	result, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	// Ensure we consumed all tokens.
	if p.peek().typ != tokEOF {
		return nil, fmt.Errorf("unexpected token %q after expression", p.peek().val)
	}

	return &ParseResult{
		SQL:       result,
		Refs:      p.refSlugs,
		CrossRefs: p.crossRefs,
	}, nil
}

// parseExpr handles the lowest-precedence operators: comparison (<, >, <=, >=, =, !=, <>).
// Precedence levels (lowest to highest): comparison -> add/sub -> mul/div -> unary -> primary.
func (p *Parser) parseExpr() (string, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return "", err
	}

	for {
		t := p.peek()
		if t.typ == tokOperator && (t.val == "<" || t.val == ">" || t.val == "<=" || t.val == ">=" || t.val == "=" || t.val == "!=" || t.val == "<>") {
			p.advance()
			op := t.val
			if op == "=" {
				op = "=" // SQL uses single =
			}
			if op == "!=" {
				op = "<>"
			}
			right, err := p.parseAddSub()
			if err != nil {
				return "", err
			}
			left = fmt.Sprintf("(%s %s %s)", left, op, right)
		} else {
			break
		}
	}
	return left, nil
}

// parseAddSub handles + and -.
func (p *Parser) parseAddSub() (string, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return "", err
	}

	for {
		t := p.peek()
		if t.typ == tokOperator && (t.val == "+" || t.val == "-") {
			p.advance()
			right, err := p.parseMulDiv()
			if err != nil {
				return "", err
			}
			left = fmt.Sprintf("(%s %s %s)", left, t.val, right)
		} else {
			break
		}
	}
	return left, nil
}

// parseMulDiv handles *, /, %.
func (p *Parser) parseMulDiv() (string, error) {
	left, err := p.parseUnary()
	if err != nil {
		return "", err
	}

	for {
		t := p.peek()
		if t.typ == tokOperator && (t.val == "*" || t.val == "/" || t.val == "%") {
			p.advance()
			right, err := p.parseUnary()
			if err != nil {
				return "", err
			}
			left = fmt.Sprintf("(%s %s %s)", left, t.val, right)
		} else {
			break
		}
	}
	return left, nil
}

// parseUnary handles unary - and +.
func (p *Parser) parseUnary() (string, error) {
	t := p.peek()
	if t.typ == tokOperator && (t.val == "-" || t.val == "+") {
		p.advance()
		operand, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		if t.val == "-" {
			return fmt.Sprintf("(-%s)", operand), nil
		}
		return operand, nil
	}
	return p.parsePrimary()
}

// parsePrimary handles atoms: numeric literals, string literals, identifiers (field slugs
// or function calls), and parenthesized sub-expressions. Identifiers are checked against
// the allowed function whitelist first, then cross-collection functions, then validated
// as field slugs. Unknown identifiers cause an error.
func (p *Parser) parsePrimary() (string, error) {
	t := p.peek()

	switch t.typ {
	case tokNumber:
		p.advance()
		// Validate it's a real number.
		if _, err := strconv.ParseFloat(t.val, 64); err != nil {
			return "", fmt.Errorf("invalid number %q", t.val)
		}
		return t.val, nil

	case tokString:
		p.advance()
		// Escape single quotes for SQL.
		escaped := strings.ReplaceAll(t.val, "'", "''")
		return fmt.Sprintf("'%s'", escaped), nil

	case tokIdent:
		p.advance()
		upper := strings.ToUpper(t.val)

		// IF function → CASE WHEN ... THEN ... ELSE ... END
		if upper == "IF" {
			return p.parseIF()
		}

		// Cross-collection functions.
		if crossCollectionFunctions[upper] {
			return p.parseCrossCollectionFunc(upper)
		}

		// Known function call.
		if allowedFunctions[upper] {
			return p.parseFunctionCall(upper)
		}

		// Must be a field slug reference.
		if !p.slugs[t.val] {
			return "", fmt.Errorf("unknown field or function %q", t.val)
		}
		p.refSlugs = append(p.refSlugs, t.val)
		return pgutil.QuoteIdent(t.val), nil

	case tokLParen:
		p.advance()
		expr, err := p.parseExpr()
		if err != nil {
			return "", err
		}
		if _, err := p.expect(tokRParen); err != nil {
			return "", fmt.Errorf("expected closing parenthesis")
		}
		return fmt.Sprintf("(%s)", expr), nil

	default:
		return "", fmt.Errorf("unexpected token %q", t.val)
	}
}

// parseFunctionCall parses a whitelisted function call: FUNC(arg1, arg2, ...).
// Only functions in allowedFunctions are routed here. The result is emitted
// directly as SQL (e.g. "ROUND(col, 2)") since only safe function names are allowed.
func (p *Parser) parseFunctionCall(name string) (string, error) {
	if _, err := p.expect(tokLParen); err != nil {
		return "", fmt.Errorf("expected '(' after function %s", name)
	}

	var args []string
	if p.peek().typ != tokRParen {
		for {
			arg, err := p.parseExpr()
			if err != nil {
				return "", err
			}
			args = append(args, arg)
			if p.peek().typ == tokComma {
				p.advance()
			} else {
				break
			}
		}
	}

	if _, err := p.expect(tokRParen); err != nil {
		return "", fmt.Errorf("expected ')' after function arguments")
	}

	return fmt.Sprintf("%s(%s)", name, strings.Join(args, ", ")), nil
}

// parseIF converts IF(cond, then, else) → CASE WHEN cond THEN then ELSE else END.
func (p *Parser) parseIF() (string, error) {
	if _, err := p.expect(tokLParen); err != nil {
		return "", fmt.Errorf("expected '(' after IF")
	}

	cond, err := p.parseExpr()
	if err != nil {
		return "", fmt.Errorf("IF condition: %w", err)
	}
	if p.peek().typ != tokComma {
		return "", fmt.Errorf("expected ',' after IF condition")
	}
	p.advance()

	thenExpr, err := p.parseExpr()
	if err != nil {
		return "", fmt.Errorf("IF then: %w", err)
	}
	if p.peek().typ != tokComma {
		return "", fmt.Errorf("expected ',' after IF then-expression")
	}
	p.advance()

	elseExpr, err := p.parseExpr()
	if err != nil {
		return "", fmt.Errorf("IF else: %w", err)
	}

	if _, err := p.expect(tokRParen); err != nil {
		return "", fmt.Errorf("expected ')' after IF else-expression")
	}

	return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", cond, thenExpr, elseExpr), nil
}

// parseCrossCollectionFunc handles LOOKUP, SUMREL, AVGREL, MINREL, MAXREL, COUNTREL.
//
// Syntax:
//
//	LOOKUP(relation_field, target_field)
//	  → (SELECT "target_field" FROM target_table WHERE id = "relation_field")
//
//	SUMREL(relation_field, target_field)
//	  → (SELECT COALESCE(SUM("target_field"), 0) FROM target_table WHERE "reverse_col" = "id")
func (p *Parser) parseCrossCollectionFunc(name string) (string, error) {
	if p.resolver == nil {
		return "", fmt.Errorf("%s requires cross-collection support (resolver not available)", name)
	}

	if _, err := p.expect(tokLParen); err != nil {
		return "", fmt.Errorf("expected '(' after %s", name)
	}

	// First argument: relation field slug.
	relTok := p.advance()
	if relTok.typ != tokIdent {
		return "", fmt.Errorf("%s: first argument must be a relation field slug", name)
	}
	relSlug := relTok.val

	if p.peek().typ != tokComma {
		return "", fmt.Errorf("expected ',' after relation field in %s", name)
	}
	p.advance()

	// Second argument: target field slug.
	targetTok := p.advance()
	if targetTok.typ != tokIdent {
		return "", fmt.Errorf("%s: second argument must be a target field slug", name)
	}
	targetField := targetTok.val

	if _, err := p.expect(tokRParen); err != nil {
		return "", fmt.Errorf("expected ')' after %s arguments", name)
	}

	// Resolve the relation.
	info, err := p.resolver(relSlug)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}

	p.crossRefs = append(p.crossRefs, relSlug)

	// Validate target_field is a safe identifier.
	if !isValidIdent(targetField) {
		return "", fmt.Errorf("%s: invalid target field %q", name, targetField)
	}
	qTargetField := pgutil.QuoteIdent(targetField)

	switch name {
	case "LOOKUP":
		// Scalar subquery: follow FK to get a single value.
		return fmt.Sprintf(
			`(SELECT %s FROM %s WHERE id = %q)`,
			qTargetField, info.TargetTable, relSlug,
		), nil

	case "SUMREL", "AVGREL", "MINREL", "MAXREL", "COUNTREL":
		aggFn := strings.TrimSuffix(name, "REL")
		if info.ReverseColumn == "" {
			return "", fmt.Errorf("%s: no reverse relation found for %q", name, relSlug)
		}
		if aggFn == "COUNT" {
			return fmt.Sprintf(
				`(SELECT COUNT(*) FROM %s WHERE %q = id)`,
				info.TargetTable, info.ReverseColumn,
			), nil
		}
		return fmt.Sprintf(
			`(SELECT COALESCE(%s(%s), 0) FROM %s WHERE %q = id)`,
			aggFn, qTargetField, info.TargetTable, info.ReverseColumn,
		), nil

	default:
		return "", fmt.Errorf("unknown cross-collection function %q", name)
	}
}

func isValidIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, ch := range s {
		if i == 0 && !unicode.IsLetter(ch) && ch != '_' {
			return false
		}
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return false
		}
	}
	return true
}
