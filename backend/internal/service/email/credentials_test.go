package email

import (
	"errors"
	"testing"
)

func TestNormalizeAppPassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  error
		want     string
	}{
		{name: "spaced groups of four", password: "abcd efgh ijkl mnop", want: "abcdefghijklmnop"},
		{name: "tabbed", password: "abcd\tefgh\tijkl\tmnop", want: "abcdefghijklmnop"},
		{name: "15 characters after stripping", password: "abcd efgh ijkl mno", wantErr: ErrInvalidAppPassword},
		{name: "17 characters after stripping", password: "abcd efgh ijkl mnopq", wantErr: ErrInvalidAppPassword},
		{name: "empty", password: "", wantErr: ErrInvalidAppPassword},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalize(Credentials{Address: "user@example.com", AppPassword: tc.password})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.AppPassword != tc.want {
				t.Errorf("password = %q, want %q", got.AppPassword, tc.want)
			}
		})
	}
}

func TestNormalizeAddress(t *testing.T) {
	cases := []struct {
		name    string
		address string
		wantErr error
		want    string
	}{
		{name: "valid lowercase", address: "user@example.com", want: "user@example.com"},
		{name: "valid mixed case is lowercased", address: "User@Example.com", want: "user@example.com"},
		{name: "invalid, no @", address: "not-an-email", wantErr: ErrInvalidAddress},
		{name: "display name is rejected", address: "Display Name <a@b.com>", wantErr: ErrInvalidAddress},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeAddress(tc.address)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("address = %q, want %q", got, tc.want)
			}
		})
	}
}
