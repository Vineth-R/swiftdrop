package servicehttp

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/businessreg"
)

// fixtureBusinessRegNo builds a valid registration number from a UUID. The
// literal "1" guarantees the at-least-one-digit rule in businessreg.Valid even
// when every hexadecimal character in the UUID prefix is a letter.
func fixtureBusinessRegNo(prefix, id string) string {
	return prefix + "1-" + id[:7]
}

func TestFixtureBusinessRegistrationNumberAlwaysValid(t *testing.T) {
	for i := 0; i < 2000; i++ {
		id := uuid.NewString()
		value := fixtureBusinessRegNo("FIXTURE", id)
		if _, err := businessreg.NormalizeValid(value); err != nil {
			t.Fatalf("fixture registration number %q is invalid: %v", value, err)
		}
	}
}
