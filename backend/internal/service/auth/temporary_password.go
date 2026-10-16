package auth

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"
)

const (
	// temporaryPasswordAlphabet has 31 symbols: the digits and capitals left
	// once the look-alikes 0, 1, I, L and O are dropped, so a password read
	// from an email or a printout cannot be mistyped.
	temporaryPasswordAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	temporaryPasswordSymbols  = 20
	temporaryPasswordGroup    = 5
	// temporaryPasswordAcceptBelow is the largest multiple of the alphabet
	// size that fits in a byte. Bytes at or above it are discarded, because
	// reducing them modulo the alphabet size would favour the first symbols.
	temporaryPasswordAcceptBelow = 256 - 256%len(temporaryPasswordAlphabet)
)

// GenerateTemporaryPassword returns 20 random symbols grouped as
// XXXXX-XXXXX-XXXXX-XXXXX, about 99 bits of entropy.
func GenerateTemporaryPassword() (string, error) {
	return generateTemporaryPassword(rand.Reader)
}

func generateTemporaryPassword(source io.Reader) (string, error) {
	var password strings.Builder
	password.Grow(temporaryPasswordSymbols + temporaryPasswordSymbols/temporaryPasswordGroup)
	buf := make([]byte, temporaryPasswordSymbols)
	for symbols := 0; symbols < temporaryPasswordSymbols; {
		if _, err := io.ReadFull(source, buf[:temporaryPasswordSymbols-symbols]); err != nil {
			return "", fmt.Errorf("generate temporary password: %w", err)
		}
		for _, b := range buf[:temporaryPasswordSymbols-symbols] {
			if int(b) >= temporaryPasswordAcceptBelow {
				continue
			}
			if symbols > 0 && symbols%temporaryPasswordGroup == 0 {
				password.WriteByte('-')
			}
			password.WriteByte(temporaryPasswordAlphabet[int(b)%len(temporaryPasswordAlphabet)])
			symbols++
		}
	}
	return password.String(), nil
}
