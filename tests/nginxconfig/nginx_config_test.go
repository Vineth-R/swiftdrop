package nginxconfig

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func configText(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	content, err := os.ReadFile(filepath.Join(root, "infra", "nginx", "default.conf"))
	if err != nil {
		t.Fatalf("read default.conf: %v", err)
	}
	return string(content)
}

// blockAfter returns the brace-delimited block that starts at marker.
func blockAfter(text, marker string) string {
	start := strings.Index(text, marker)
	if start < 0 {
		return ""
	}
	open := strings.Index(text[start:], "{")
	if open < 0 {
		return ""
	}
	depth := 0
	for i := start + open; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

func TestNginxAuthAccessLogOmitsQueryString(t *testing.T) {
	text := configText(t)

	if strings.Count(text, "log_format swiftdrop_no_query") != 1 {
		t.Fatal("expected exactly one swiftdrop_no_query log_format")
	}
	start := strings.Index(text, "log_format swiftdrop_no_query")
	end := strings.Index(text[start:], ";")
	if start < 0 || end < 0 {
		t.Fatal("cannot read the swiftdrop_no_query format")
	}
	format := text[start : start+end+1]
	if !strings.Contains(format, "$uri") {
		t.Fatalf("log format does not contain $uri: %q", format)
	}
	if strings.Contains(format, "$request_uri") {
		t.Fatalf("log format contains $request_uri: %q", format)
	}
	if strings.Contains(format, "$args") {
		t.Fatalf("log format contains $args: %q", format)
	}
	if regexp.MustCompile(`\$request([^_a-zA-Z0-9]|$)`).MatchString(format) {
		t.Fatalf("log format contains the raw $request variable: %q", format)
	}

	auth := blockAfter(text, "location /api/v1/auth/")
	if auth == "" {
		t.Fatal("cannot find the /api/v1/auth/ location")
	}
	if !strings.Contains(auth, "access_log /var/log/nginx/access.log swiftdrop_no_query;") {
		t.Fatalf("/api/v1/auth/ does not use swiftdrop_no_query: %q", auth)
	}
	if strings.Contains(auth, "access_log off") {
		t.Fatal("/api/v1/auth/ must keep authentication logging enabled without query strings")
	}
}

func TestNginxUnrelatedRoutesKeepDefaultLogging(t *testing.T) {
	text := configText(t)

	parcels := blockAfter(text, "location /api/v1/parcels/")
	if parcels == "" {
		t.Fatal("cannot find the /api/v1/parcels/ location")
	}
	if strings.Contains(parcels, "swiftdrop_no_query") {
		t.Fatal("unrelated parcel traffic should keep the default access log")
	}

	health := blockAfter(text, "location = /nginx-health")
	if health == "" || !strings.Contains(health, "access_log off;") {
		t.Fatalf("nginx-health must keep access logging disabled: %q", health)
	}
	if strings.Count(text, "access_log off") != 1 {
		t.Fatal("only nginx-health should disable access logging")
	}
}
