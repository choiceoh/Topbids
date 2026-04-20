package jsonutil

import (
	"strings"
	"testing"
)

func TestUnmarshal(t *testing.T) {
	type item struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	data := []byte(`{"name":"alice","age":30}`)
	got, err := Unmarshal[item]("test item", data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Name != "alice" || got.Age != 30 {
		t.Errorf("got %+v, want {alice 30}", got)
	}
}

func TestUnmarshalError(t *testing.T) {
	_, err := Unmarshal[map[string]string]("config", []byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Errorf("error = %q, want to contain 'parse config'", err.Error())
	}
}

func TestUnmarshalInto(t *testing.T) {
	var m map[string]int
	err := UnmarshalInto("counts", []byte(`{"a":1,"b":2}`), &m)
	if err != nil {
		t.Fatalf("UnmarshalInto error: %v", err)
	}
	if m["a"] != 1 || m["b"] != 2 {
		t.Errorf("got %v, want {a:1 b:2}", m)
	}
}

func TestUnmarshalIntoError(t *testing.T) {
	var v int
	err := UnmarshalInto("number", []byte(`"not a number"`), &v)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse number") {
		t.Errorf("error = %q, want to contain 'parse number'", err.Error())
	}
}
