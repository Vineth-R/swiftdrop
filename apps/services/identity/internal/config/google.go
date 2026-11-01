package config

import "net/url"

// GoogleProblems lists why Google sign-in cannot be enabled. An empty result
// means all three settings are present and usable. The caller logs the
// problems and leaves the Google routes unmounted rather than failing startup,
// because Google sign-in is optional (API v1.1 sections 2.5 and 2.6).
//
// The messages name settings, never their values.
func (c Config) GoogleProblems() []string {
	var problems []string
	if c.GoogleClientID == "" {
		problems = append(problems, "GOOGLE_CLIENT_ID is not set")
	}
	if c.GoogleClientSecret == "" {
		problems = append(problems, "GOOGLE_CLIENT_SECRET is not set")
	}
	switch {
	case c.GoogleRedirectURI == "":
		problems = append(problems, "GOOGLE_REDIRECT_URI is not set")
	case !absoluteHTTPURL(c.GoogleRedirectURI):
		problems = append(problems, "GOOGLE_REDIRECT_URI must be an absolute http or https URL")
	}
	return problems
}

func absoluteHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
