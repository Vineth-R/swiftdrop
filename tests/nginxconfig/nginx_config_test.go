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

func TestNginxNotificationLogRoute(t *testing.T) {
	text := configText(t)

	upstream := blockAfter(text, "upstream notification_worker_upstream")
	if upstream == "" {
		t.Fatal("cannot find the notification_worker_upstream block")
	}
	if !strings.Contains(upstream, "server notification-worker:8086;") {
		t.Fatalf("notification_worker_upstream = %q", upstream)
	}

	for _, marker := range []string{
		"location = /api/v1/admin/notification-logs",
		"location /api/v1/admin/notification-logs/",
	} {
		block := blockAfter(text, marker)
		if block == "" {
			t.Fatalf("cannot find %q", marker)
		}
		if !strings.Contains(block, "rewrite ^/api/v1/(.*)$ /$1 break;") {
			t.Fatalf("%s does not strip the /api/v1 prefix: %q", marker, block)
		}
		if !strings.Contains(block, "proxy_pass http://notification_worker_upstream;") {
			t.Fatalf("%s does not proxy to the notification worker: %q", marker, block)
		}
	}
}

func TestNginxAIChatRouteIsExact(t *testing.T) {
	text := configText(t)
	chat := blockAfter(text, "location = /api/v1/ai/chat")
	if chat == "" {
		t.Fatal("missing exact AI chat location")
	}
	if !strings.Contains(chat, "rewrite ^/api/v1/(.*)$ /$1 break;") {
		t.Fatalf("AI chat location does not strip the prefix:\n%s", chat)
	}
	if !strings.Contains(chat, "proxy_pass http://ai_assistant_upstream;") {
		t.Fatalf("AI chat location does not reach the assistant upstream:\n%s", chat)
	}

	otherAI := blockAfter(text, "location ^~ /api/v1/ai/")
	if otherAI == "" {
		t.Fatal("missing AI catch-all location")
	}
	if !strings.Contains(otherAI, "return 404") {
		t.Fatalf("AI health/other paths must not be public:\n%s", otherAI)
	}
}
