package auth

import "testing"

func TestSessionCodecRoundTripsWithAndWithoutSID(t *testing.T) {
	codec := newSessionCodec([]byte("test-key"))

	withoutSID := codec.sign(User{Email: "user@example.com", Sub: "sub-1"}, "")
	session, err := codec.verify(withoutSID)
	if err != nil {
		t.Fatalf("verify without SID: %v", err)
	}
	if session.SID != "" {
		t.Fatalf("SID = %q, want empty", session.SID)
	}

	withSID := codec.sign(User{Email: "user@example.com", Sub: "sub-1"}, "session-123")
	session, err = codec.verify(withSID)
	if err != nil {
		t.Fatalf("verify with SID: %v", err)
	}
	if session.SID != "session-123" {
		t.Fatalf("SID = %q, want %q", session.SID, "session-123")
	}
}
