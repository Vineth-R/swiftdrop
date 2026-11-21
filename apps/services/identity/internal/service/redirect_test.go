package service

import (
	"strings"
	"testing"
)

func TestValidRedirectPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "simple path", path: "/dashboard", want: true},
		{name: "nested path with query", path: "/parcels/12?tab=history", want: true},
		{name: "root", path: "/", want: true},
		{name: "empty", path: "", want: false},
		{name: "absolute https url", path: "https://evil.example", want: false},
		{name: "absolute http url", path: "http://evil.example/x", want: false},
		{name: "protocol relative", path: "//evil.example", want: false},
		{name: "protocol relative with path", path: "//evil.example/dashboard", want: false},
		{name: "backslash trick", path: `/\evil.example`, want: false},
		{name: "backslash later", path: `/a\b`, want: false},
		{name: "javascript scheme", path: "javascript:alert(1)", want: false},
		{name: "no leading slash", path: "dashboard", want: false},
		{name: "newline injection", path: "/ok\r\nSet-Cookie: x=1", want: false},
		{name: "tab", path: "/a\tb", want: false},
		{name: "space", path: "/a b", want: false},
		{name: "null byte", path: "/a\x00b", want: false},
		{name: "invalid utf-8", path: "/a\xffb", want: false},
		{name: "at the length limit", path: "/" + strings.Repeat("a", maxRedirectLength-1), want: true},
		{name: "over the length limit", path: "/" + strings.Repeat("a", maxRedirectLength), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidRedirectPath(test.path); got != test.want {
				t.Fatalf("ValidRedirectPath(%q) = %t, want %t", test.path, got, test.want)
			}
		})
	}
}
