package webpush

import "testing"

func TestVAPIDKeyRoundTripsThroughPersistedForm(t *testing.T) {
	key, err := GenerateVAPIDKey()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ParseVAPIDKey(key.PrivateKeyBase64(), key.PublicKeyBase64())
	if err != nil {
		t.Fatalf("ParseVAPIDKey() error = %v", err)
	}
	if restored.PrivateKeyBase64() != key.PrivateKeyBase64() ||
		restored.PublicKeyBase64() != key.PublicKeyBase64() {
		t.Fatal("key pair did not survive its persisted representation")
	}
}

func TestParseVAPIDKeyRejectsMismatchedPair(t *testing.T) {
	first, err := GenerateVAPIDKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateVAPIDKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseVAPIDKey(first.PrivateKeyBase64(), second.PublicKeyBase64()); err == nil {
		t.Fatal("ParseVAPIDKey() accepted a public key from another pair")
	}
}
