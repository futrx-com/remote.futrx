package auth

import (
	"context"
	"errors"
	"testing"
)

func TestLocalLoginResults(t *testing.T) {
	f := newPasswordChangeFixture(t)
	ctx := context.Background()
	user, err := f.service.LoginLocal(ctx, " Admin@Example.com ", changeTestCurrent)
	if err != nil || user.Email != changeTestEmail || user.Sub != subjectLocalAdmin {
		t.Fatalf("login = %+v, %v", user, err)
	}
	for name, c := range map[string][2]string{
		"wrong password": {changeTestEmail, "wrong password"},
		"unknown email":  {"other@example.com", changeTestCurrent},
		"both wrong":     {"other@example.com", "wrong password"},
	} {
		if _, err := f.service.LoginLocal(ctx, c[0], c[1]); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", name, err)
		}
	}
}

func TestLocalLoginWithoutAnAdministratorIsRejected(t *testing.T) {
	service := newAuthTestService(t, &authTestStore{}, newAuthTestUsers(), User{})
	if _, err := service.LoginLocal(context.Background(), changeTestEmail, changeTestCurrent); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}
