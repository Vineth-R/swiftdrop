package auth

import "testing"

func TestParseRole(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Role
		ok    bool
	}{
		{name: "customer", input: "CUSTOMER", want: RoleCustomer, ok: true},
		{name: "merchant", input: "MERCHANT", want: RoleMerchant, ok: true},
		{name: "courier", input: "COURIER", want: RoleCourier, ok: true},
		{name: "dispatcher", input: "DISPATCHER", want: RoleDispatcher, ok: true},
		{name: "admin", input: "ADMIN", want: RoleAdmin, ok: true},
		{name: "lower case is rejected", input: "admin", ok: false},
		{name: "unknown is rejected", input: "SUPERUSER", ok: false},
		{name: "empty is rejected", input: "", ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseRole(test.input)
			if ok != test.ok {
				t.Fatalf("ParseRole(%q) ok = %t, want %t", test.input, ok, test.ok)
			}
			if ok && got != test.want {
				t.Fatalf("ParseRole(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestRole_Satisfies(t *testing.T) {
	tests := []struct {
		name     string
		held     Role
		required Role
		want     bool
	}{
		{name: "same role", held: RoleCourier, required: RoleCourier, want: true},
		{name: "admin satisfies dispatcher", held: RoleAdmin, required: RoleDispatcher, want: true},
		{name: "admin satisfies admin", held: RoleAdmin, required: RoleAdmin, want: true},
		{name: "admin does not satisfy courier", held: RoleAdmin, required: RoleCourier, want: false},
		{name: "admin does not satisfy customer", held: RoleAdmin, required: RoleCustomer, want: false},
		{name: "dispatcher does not satisfy admin", held: RoleDispatcher, required: RoleAdmin, want: false},
		{name: "customer does not satisfy merchant", held: RoleCustomer, required: RoleMerchant, want: false},
		{name: "unknown held role", held: Role("ROOT"), required: RoleCustomer, want: false},
		{name: "unknown required role", held: RoleAdmin, required: Role("ROOT"), want: false},
		{name: "empty roles never match", held: Role(""), required: Role(""), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.held.Satisfies(test.required); got != test.want {
				t.Fatalf("%q.Satisfies(%q) = %t, want %t", test.held, test.required, got, test.want)
			}
		})
	}
}
