// Package accesskit checks that every endpoint refuses the callers it must
// refuse. A Case names one endpoint and the roles that may use it; Check calls
// it as an anonymous caller and as each of the five roles and reports every
// answer that breaks the rule. The same checker runs against the Compose
// gateway (tests/access) and, in the self-test, against a server with a rule
// broken on purpose, which proves a broken rule turns the suite red.
package accesskit

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// Roles lists the five SwiftDrop roles in a fixed order.
var Roles = []auth.Role{
	auth.RoleCustomer,
	auth.RoleMerchant,
	auth.RoleCourier,
	auth.RoleDispatcher,
	auth.RoleAdmin,
}

// Case is one endpoint and the roles that may call it.
type Case struct {
	// Name identifies the case in a report, for example "GET /admin/users/".
	Name string

	Method string
	Path   string
	// Body is sent as JSON when it is not empty. Use a body the handler
	// rejects (such as {}) for a write, so the check never changes data.
	Body string

	// Allowed lists the roles that must get past the access check. A role that
	// satisfies one of them (ADMIN satisfies DISPATCHER) is allowed too.
	Allowed []auth.Role

	// Pending explains why the allowed roles cannot be checked yet, for
	// example a service that does not verify access tokens yet. Callers that
	// must be refused, anonymous ones included, are still checked: they have to
	// get a 401 or 403, never data.
	Pending string

	// Missing explains why the endpoint cannot be called at all yet, because
	// its service has not built it. Check skips the case. The owner removes the
	// field when the endpoint exists, and the case starts running.
	Missing string
}

// Gateway is where the cases are sent and the tokens to send them with.
type Gateway struct {
	BaseURL string
	Client  *http.Client
	// Tokens holds one valid access token per role.
	Tokens map[auth.Role]string
}

// Violation is one answer that breaks an access rule.
type Violation struct {
	Case   string
	Actor  string
	Status int
	Want   string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s as %s: got %d, want %s", v.Case, v.Actor, v.Status, v.Want)
}

// Allows reports whether role may call the case's endpoint.
func (c Case) Allows(role auth.Role) bool {
	for _, allowed := range c.Allowed {
		if role.Satisfies(allowed) {
			return true
		}
	}
	return false
}

// Split separates the cases Check will run from the ones whose endpoint does not
// exist yet.
func Split(cases []Case) (active, missing []Case) {
	for _, c := range cases {
		if c.Missing != "" {
			missing = append(missing, c)
		} else {
			active = append(active, c)
		}
	}
	return active, missing
}

// Check sends every case as an anonymous caller and as each role, and returns
// the answers that break a rule. Cases marked Missing are skipped. The error is
// for a failure to reach the gateway, not for a violation.
func Check(gateway Gateway, cases []Case) ([]Violation, error) {
	client := gateway.Client
	if client == nil {
		client = &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}

	var violations []Violation
	cases, _ = Split(cases)
	for _, c := range cases {
		anonymous, err := send(client, gateway.BaseURL, c, "")
		if err != nil {
			return violations, err
		}
		if want, ok := judgeAnonymous(c, anonymous); !ok {
			violations = append(violations, Violation{c.Name, "anonymous", anonymous, want})
		}

		for _, role := range Roles {
			token, ok := gateway.Tokens[role]
			if !ok {
				return violations, fmt.Errorf("no access token for role %s", role)
			}
			status, err := send(client, gateway.BaseURL, c, token)
			if err != nil {
				return violations, err
			}
			if want, ok := judge(c, role, status); !ok {
				violations = append(violations, Violation{c.Name, string(role), status, want})
			}
		}
	}
	return violations, nil
}

// judgeAnonymous expects a 401 for a caller with no token. A pending case only
// needs the refusal, because its service may not tell "no token" from "wrong
// role" yet.
func judgeAnonymous(c Case, status int) (want string, ok bool) {
	if c.Pending != "" {
		return "401 or 403", status == http.StatusUnauthorized || status == http.StatusForbidden
	}
	return "401", status == http.StatusUnauthorized
}

func judge(c Case, role auth.Role, status int) (want string, ok bool) {
	refused := status == http.StatusUnauthorized || status == http.StatusForbidden
	switch {
	case c.Allows(role) && c.Pending != "":
		return "", true
	case c.Allows(role):
		return "an answer from the handler (not 401 or 403)", !refused
	case c.Pending != "":
		return "401 or 403", refused
	default:
		return "403", status == http.StatusForbidden
	}
}

func send(client *http.Client, base string, c Case, token string) (int, error) {
	var body io.Reader
	if c.Body != "" {
		body = strings.NewReader(c.Body)
	}
	request, err := http.NewRequest(c.Method, strings.TrimRight(base, "/")+c.Path, body)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", c.Name, err)
	}
	if c.Body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", c.Name, err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, nil
}
