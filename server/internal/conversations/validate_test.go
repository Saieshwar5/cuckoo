package conversations

import (
	"strings"
	"testing"
)

func TestValidateText(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
		ok   bool
	}{
		"plain":            {"hello", "hello", true},
		"trimmed":          {"  hello \n", "hello", true},
		"multiline kept":   {"line one\nline two", "line one\nline two", true},
		"at limit":         {strings.Repeat("a", 8000), strings.Repeat("a", 8000), true},
		"over limit":       {strings.Repeat("a", 8001), "", false},
		"runes not bytes":  {strings.Repeat("क", 8000), strings.Repeat("क", 8000), true},
		"empty":            {"", "", false},
		"whitespace":       {" \t\n", "", false},
		"nul":              {"a\x00b", "", false},
		"invalid encoding": {"a\xffb", "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := validateText(tc.in)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPageSize(t *testing.T) {
	cases := map[int]struct {
		want int32
		ok   bool
	}{0: {50, true}, 1: {1, true}, 100: {100, true}, 101: {0, false}, -1: {0, false}}
	for in, tc := range cases {
		got, err := pageSize(in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("pageSize(%d) = %d, %v; want %d, ok=%v", in, got, err, tc.want, tc.ok)
		}
	}
}
