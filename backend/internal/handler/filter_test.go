package handler

import (
	"net/url"
	"strings"
	"testing"

	"github.com/choiceoh/topbid/backend/internal/schema"
)

func sampleFields() []schema.Field {
	return []schema.Field{
		{Slug: "name", FieldType: schema.FieldText},
		{Slug: "capacity", FieldType: schema.FieldNumber},
		{Slug: "status", FieldType: schema.FieldSelect},
	}
}

func TestParseFiltersEq(t *testing.T) {
	params := url.Values{"status": []string{"eq:active"}}
	where, args, err := ParseFilters(params, sampleFields())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(where, `"status" = $1`) {
		t.Errorf("missing status condition: %s", where)
	}
	if len(args) != 1 || args[0] != "active" {
		t.Errorf("args = %v", args)
	}
}

func TestParseFiltersMultipleOps(t *testing.T) {
	params := url.Values{
		"capacity": []string{"gte:30"},
		"name":     []string{"like:여수"},
	}
	where, args, err := ParseFilters(params, sampleFields())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(where, `"capacity" >= $`) {
		t.Errorf("missing >= condition: %s", where)
	}
	if !strings.Contains(where, `"name" ILIKE $`) {
		t.Errorf("missing ILIKE condition: %s", where)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
	// like wraps with %
	foundLike := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "%여수%" {
			foundLike = true
		}
	}
	if !foundLike {
		t.Errorf("like value not wrapped: %v", args)
	}
}

func TestParseFiltersIn(t *testing.T) {
	params := url.Values{"status": []string{"in:a,b,c"}}
	where, args, err := ParseFilters(params, sampleFields())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(where, `"status" IN ($1,$2,$3)`) {
		t.Errorf("got: %s", where)
	}
	if len(args) != 3 {
		t.Errorf("args = %v", args)
	}
}

func TestParseFiltersIsNull(t *testing.T) {
	params := url.Values{"capacity": []string{"is_null:true"}}
	where, _, err := ParseFilters(params, sampleFields())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(where, `"capacity" IS NULL`) {
		t.Errorf("got: %s", where)
	}
}

func TestParseFiltersUnknownFieldIgnored(t *testing.T) {
	params := url.Values{"nonexistent": []string{"eq:foo"}}
	where, args, err := ParseFilters(params, sampleFields())
	if err != nil {
		t.Fatal(err)
	}
	if where != "" || len(args) != 0 {
		t.Errorf("unknown field should be silently ignored: where=%q args=%v", where, args)
	}
}

func TestParseFiltersReservedParamsSkipped(t *testing.T) {
	params := url.Values{
		"sort":   []string{"-name"},
		"page":   []string{"2"},
		"limit":  []string{"10"},
		"expand": []string{"subsidiary"},
		"name":   []string{"eq:foo"},
	}
	where, args, err := ParseFilters(params, sampleFields())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(where, `"name" = $1`) || len(args) != 1 {
		t.Errorf("only name should be a filter: where=%q args=%v", where, args)
	}
}

func TestParseFiltersInvalidFormat(t *testing.T) {
	params := url.Values{"name": []string{"no_colon_here"}}
	_, _, err := ParseFilters(params, sampleFields())
	if err == nil {
		t.Error("expected error for malformed filter")
	}
}

func TestParseFiltersUnknownOperator(t *testing.T) {
	params := url.Values{"name": []string{"contains:foo"}}
	_, _, err := ParseFilters(params, sampleFields())
	if err == nil {
		t.Error("expected error for unknown operator")
	}
}

func TestParseSort(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "ORDER BY created_at DESC"},
		{"name", `ORDER BY "name" ASC`},
		{"-name", `ORDER BY "name" DESC`},
		{"name,-capacity", `ORDER BY "name" ASC, "capacity" DESC`},
		{"unknown", "ORDER BY created_at DESC"}, // unknown ignored
	}
	for _, tc := range cases {
		got := ParseSort(tc.in, sampleFields())
		if got != tc.want {
			t.Errorf("ParseSort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParsePagination(t *testing.T) {
	cases := []struct {
		params              url.Values
		wantPage, wantLimit int
		wantOffset          int
	}{
		{url.Values{}, 1, 20, 0},
		{url.Values{"page": []string{"3"}}, 3, 20, 40},
		{url.Values{"limit": []string{"50"}}, 1, 50, 0},
		{url.Values{"page": []string{"2"}, "limit": []string{"10"}}, 2, 10, 10},
		{url.Values{"page": []string{"-1"}}, 1, 20, 0},    // negative page → default
		{url.Values{"limit": []string{"1000"}}, 1, 20, 0}, // > max → default
		{url.Values{"limit": []string{"abc"}}, 1, 20, 0},  // non-numeric → default
	}
	for _, tc := range cases {
		page, limit, offset := ParsePagination(tc.params)
		if page != tc.wantPage || limit != tc.wantLimit || offset != tc.wantOffset {
			t.Errorf("params=%v: got (%d,%d,%d), want (%d,%d,%d)",
				tc.params, page, limit, offset, tc.wantPage, tc.wantLimit, tc.wantOffset)
		}
	}
}
