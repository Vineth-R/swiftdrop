package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestEscapeLike(t *testing.T) {
	tests := map[string]string{
		"nimal":    "nimal",
		"100%":     `100\%`,
		"a_b":      `a\_b`,
		`back\sl`:  `back\\sl`,
		"%_\\":     `\%\_\\`,
		"o'brien":  "o'brien",
		"":         "",
		"x' OR 1=": "x' OR 1=",
	}
	for input, want := range tests {
		if got := escapeLike(input); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUserWhere(t *testing.T) {
	active := false

	t.Run("no filters means no WHERE", func(t *testing.T) {
		where, args := userWhere(UserFilter{})
		if where != "" || len(args) != 0 {
			t.Fatalf("where = %q args = %v", where, args)
		}
	})

	t.Run("every filter becomes a numbered parameter", func(t *testing.T) {
		where, args := userWhere(UserFilter{Role: auth.RoleCourier, IsActive: &active, Search: "kas_un"})
		want := `WHERE r.name = $1 AND u.is_active = $2 AND (u.full_name ILIKE $3 ESCAPE '\' OR u.email ILIKE $3 ESCAPE '\')`
		if strings.TrimSpace(where) != want {
			t.Fatalf("where = %q\nwant    %q", where, want)
		}
		if len(args) != 3 || args[0] != "COURIER" || args[1] != false || args[2] != `%kas\_un%` {
			t.Fatalf("args = %v", args)
		}
	})

	t.Run("values never appear in the SQL text", func(t *testing.T) {
		hostile := "'; DROP TABLE identity.users; --"
		where, args := userWhere(UserFilter{Search: hostile, Role: auth.Role(hostile)})
		if strings.Contains(where, "DROP") || strings.Contains(where, hostile) {
			t.Fatalf("user input reached the SQL text: %q", where)
		}
		if len(args) != 2 {
			t.Fatalf("args = %v", args)
		}
	})
}

func TestSortColumns_areAFixedWhitelist(t *testing.T) {
	for field, column := range sortColumns {
		if strings.ContainsAny(column, " ;'\"") && !strings.HasPrefix(column, "lower(") {
			t.Errorf("sort column for %q looks unsafe: %q", field, column)
		}
	}
	for _, field := range []string{"createdAt", "fullName", "email"} {
		if _, ok := sortColumns[field]; !ok {
			t.Errorf("missing sort field %q", field)
		}
	}
	for _, field := range []string{"password_hash", "id; DROP TABLE x", ""} {
		if _, ok := sortColumns[field]; ok {
			t.Errorf("%q must not be sortable", field)
		}
	}
}

func TestChangedFields(t *testing.T) {
	name, phone := "n", "p"
	role := auth.RoleCourier
	got := changedFields(UserChange{FullName: &name, Phone: &phone, Role: &role, Courier: &CourierProfileChange{}})
	if strings.Join(got, ",") != "fullName,phone,role,courierProfile" {
		t.Fatalf("changedFields() = %v", got)
	}
	if got := changedFields(UserChange{}); got == nil || len(got) != 0 {
		t.Fatalf("changedFields() for no change = %#v, want an empty slice", got)
	}
}

func TestRepository_AdminUserByID_errorMapping(t *testing.T) {
	boom := errors.New("connection reset")

	repo, _ := New(&fakeDB{row: errRow{err: pgxNoRows()}})
	if _, err := repo.AdminUserByID(context.Background(), "id"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	repo, _ = New(&fakeDB{row: errRow{err: boom}})
	if err := func() error { _, err := repo.AdminUserByID(context.Background(), "id"); return err }(); !errors.Is(err, boom) || errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want the database error", err)
	}
}

// errRow is a pgx.Row whose Scan fails.
type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

func pgxNoRows() error { return pgx.ErrNoRows }
