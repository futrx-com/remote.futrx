package webpush

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	protocol "github.com/SherClockHolmes/webpush-go"
)

const (
	vapidPrivateKeyLength = 32
	vapidPublicKeyLength  = 65
)

// VAPIDKey is the server's long-lived application key pair. Both halves remain
// in the same base64url representation already persisted by Remote.
type VAPIDKey struct {
	private string
	public  string
}

// GenerateVAPIDKey mints a key pair through the Web Push implementation.
func GenerateVAPIDKey() (VAPIDKey, error) {
	privateKey, publicKey, err := protocol.GenerateVAPIDKeys()
	if err != nil {
		return VAPIDKey{}, fmt.Errorf("generate vapid key: %w", err)
	}
	return ParseVAPIDKey(privateKey, publicKey)
}

// ParseVAPIDKey validates a persisted key pair and verifies that its public
// half actually belongs to its private half before it is handed to webpush-go.
func ParseVAPIDKey(privateBase64, publicBase64 string) (VAPIDKey, error) {
	privateRaw, err := decodeVAPIDKey(privateBase64)
	if err != nil {
		return VAPIDKey{}, fmt.Errorf("decode vapid private key: %w", err)
	}
	if len(privateRaw) != vapidPrivateKeyLength {
		return VAPIDKey{}, fmt.Errorf("vapid private key must be %d bytes, got %d", vapidPrivateKeyLength, len(privateRaw))
	}
	privateKey, err := ecdh.P256().NewPrivateKey(privateRaw)
	if err != nil {
		return VAPIDKey{}, fmt.Errorf("invalid vapid private key: %w", err)
	}

	publicRaw, err := decodeVAPIDKey(publicBase64)
	if err != nil {
		return VAPIDKey{}, fmt.Errorf("decode vapid public key: %w", err)
	}
	if len(publicRaw) != vapidPublicKeyLength {
		return VAPIDKey{}, fmt.Errorf("vapid public key must be %d bytes, got %d", vapidPublicKeyLength, len(publicRaw))
	}
	if _, err := ecdh.P256().NewPublicKey(publicRaw); err != nil {
		return VAPIDKey{}, fmt.Errorf("invalid vapid public key: %w", err)
	}
	if !bytes.Equal(privateKey.PublicKey().Bytes(), publicRaw) {
		return VAPIDKey{}, errors.New("vapid public key does not match private key")
	}

	return VAPIDKey{
		private: base64.RawURLEncoding.EncodeToString(privateRaw),
		public:  base64.RawURLEncoding.EncodeToString(publicRaw),
	}, nil
}

func (k VAPIDKey) PublicKeyBase64() string  { return k.public }
func (k VAPIDKey) PrivateKeyBase64() string { return k.private }
func (k VAPIDKey) valid() bool              { return k.private != "" && k.public != "" }

func decodeVAPIDKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	for _, encoding := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("key is not valid base64")
}
