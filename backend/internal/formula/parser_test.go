package formula

import (
	"fmt"
	"testing"
)

func TestParse(t *testing.T) {
	slugs := map[string]bool{
		"price":    true,
		"quantity": true,
		"tax":      true,
		"discount": true,
		"name":     true,
	}

	tests := []struct {
		name    string
		input   string
		wantSQL string
		wantErr bool
	}{
		{
			name:    "simple multiplication",
			input:   "price * quantity",
			wantSQL: `("price" * "quantity")`,
		},
		{
			name:    "leading equals sign",
			input:   "=price * quantity",
			wantSQL: `("price" * "quantity")`,
		},
		{
			name:    "arithmetic with parentheses",
			input:   "(price + tax) * quantity",
			wantSQL: `((("price" + "tax")) * "quantity")`,
		},
		{
			name:    "ROUND function",
			input:   "ROUND(price * 1.1, 2)",
			wantSQL: `ROUND(("price" * 1.1), 2)`,
		},
		{
			name:    "IF function",
			input:   "IF(quantity > 100, price * 0.9, price)",
			wantSQL: `CASE WHEN ("quantity" > 100) THEN ("price" * 0.9) ELSE "price" END`,
		},
		{
			name:    "COALESCE",
			input:   "COALESCE(discount, 0)",
			wantSQL: `COALESCE("discount", 0)`,
		},
		{
			name:    "nested functions",
			input:   "ROUND(price * quantity - COALESCE(discount, 0), 2)",
			wantSQL: `ROUND((("price" * "quantity") - COALESCE("discount", 0)), 2)`,
		},
		{
			name:    "unary minus",
			input:   "-price",
			wantSQL: `(-"price")`,
		},
		{
			name:    "number literal",
			input:   "price + 100",
			wantSQL: `("price" + 100)`,
		},
		{
			name:    "string literal",
			input:   "COALESCE(name, 'unknown')",
			wantSQL: `COALESCE("name", 'unknown')`,
		},
		{
			name:    "modulo",
			input:   "quantity % 10",
			wantSQL: `("quantity" % 10)`,
		},
		{
			name:    "unknown field",
			input:   "unknown_field * 2",
			wantErr: true,
		},
		{
			name:    "unknown function",
			input:   "DROP(price)",
			wantErr: true,
		},
		{
			name:    "empty expression",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, _, err := Parse(tt.input, slugs)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got sql=%q", sql)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("sql mismatch:\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
		})
	}
}

func TestParseRefs(t *testing.T) {
	slugs := map[string]bool{"price": true, "quantity": true, "tax": true}
	_, refs, err := Parse("price * quantity + tax", slugs)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 {
		t.Errorf("expected 3 refs, got %d: %v", len(refs), refs)
	}
}

func TestParseCrossCollection(t *testing.T) {
	slugs := map[string]bool{"customer": true, "price": true}

	resolver := func(relSlug string) (*RelationInfo, error) {
		switch relSlug {
		case "customer":
			return &RelationInfo{
				TargetTable:   `"data"."customers"`,
				OwnerColumn:   "customer",
				ReverseColumn: "order_id",
			}, nil
		default:
			return nil, fmt.Errorf("unknown relation %q", relSlug)
		}
	}

	tests := []struct {
		name    string
		input   string
		wantSQL string
		wantErr bool
	}{
		{
			name:    "LOOKUP",
			input:   "LOOKUP(customer, name)",
			wantSQL: `(SELECT "name" FROM "data"."customers" WHERE id = "customer")`,
		},
		{
			name:    "SUMREL",
			input:   "SUMREL(customer, amount)",
			wantSQL: `(SELECT COALESCE(SUM("amount"), 0) FROM "data"."customers" WHERE "order_id" = id)`,
		},
		{
			name:    "COUNTREL",
			input:   "COUNTREL(customer, id)",
			wantSQL: `(SELECT COUNT(*) FROM "data"."customers" WHERE "order_id" = id)`,
		},
		{
			name:    "LOOKUP in arithmetic",
			input:   "price * LOOKUP(customer, discount_rate)",
			wantSQL: `("price" * (SELECT "discount_rate" FROM "data"."customers" WHERE id = "customer"))`,
		},
		{
			name:    "unknown relation",
			input:   "LOOKUP(unknown_rel, name)",
			wantErr: true,
		},
		{
			name:    "no resolver",
			input:   "LOOKUP(customer, name)",
			wantErr: true, // tested without resolver below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var res RelationResolver
			if tt.name != "no resolver" {
				res = resolver
			}
			result, err := ParseWithResolver(tt.input, slugs, res)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got sql=%q", result.SQL)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.SQL != tt.wantSQL {
				t.Errorf("sql mismatch:\n  got:  %s\n  want: %s", result.SQL, tt.wantSQL)
			}
		})
	}
}
