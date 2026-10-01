package auth

import (
	"context"
	"testing"
)

// The permission layer treats Service.IsAdmin as its root policy, so both
// administrator sources it recognizes are pinned here.
func TestIsAdminRecognizesLocalAndDirectoryAdministrators(t *testing.T) {
	users := newAuthTestUsers()
	users.roles["directory-admin@example.com"] = true
	users.roles["member@example.com"] = false
	store := &authTestStore{local: &LocalAdminCredential{Email: "local-admin@example.com"}}
	service := newAuthTestService(t, store, users, User{})

	tests := []struct {
		name  string
		email string
		admin bool
	}{
		{"local administrator", "local-admin@example.com", true},
		{"local administrator ignores case and padding", "  Local-Admin@Example.com ", true},
		{"directory administrator", "directory-admin@example.com", true},
		{"registered member", "member@example.com", false},
		{"unknown account", "stranger@example.com", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := service.IsAdmin(context.Background(), test.email)
			if err != nil {
				t.Fatalf("IsAdmin() error = %v", err)
			}
			if got != test.admin {
				t.Fatalf("IsAdmin(%q) = %v, want %v", test.email, got, test.admin)
			}
		})
	}
}

func TestIsRegisteredIncludesLocalAdministrator(t *testing.T) {
	users := newAuthTestUsers()
	users.roles["member@example.com"] = false
	store := &authTestStore{local: &LocalAdminCredential{Email: "local-admin@example.com"}}
	service := newAuthTestService(t, store, users, User{})

	for email, want := range map[string]bool{
		"local-admin@example.com": true,
		"member@example.com":      true,
		"stranger@example.com":    false,
	} {
		got, err := service.IsRegistered(context.Background(), email)
		if err != nil {
			t.Fatalf("IsRegistered(%q) error = %v", email, err)
		}
		if got != want {
			t.Fatalf("IsRegistered(%q) = %v, want %v", email, got, want)
		}
	}
}
