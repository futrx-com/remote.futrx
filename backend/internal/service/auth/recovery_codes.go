package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// recoveryCodeAlphabet excludes visually ambiguous characters (0/O, 1/I/L)
// so codes are easy to transcribe by hand.
const recoveryCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// recoveryCodeGroups/recoveryCodeGroupLen produce codes like
// "ABCD-EFGH-JKMN" - 12 alphabet characters, each drawn from a 32-symbol
// alphabet, for 12*log2(32) = 60 bits of entropy.
const (
	recoveryCodeGroups   = 3
	recoveryCodeGroupLen = 4
)

// generateRecoveryCodes returns n freshly generated, unique, grouped
// recovery codes (e.g. "ABCD-EFGH-JKMN"), each carrying ~60 bits of
// server-chosen entropy.
func generateRecoveryCodes(n int) ([]string, error) {
	codes := make([]string, 0, n)
	seen := make(map[string]struct{}, n)
	for len(codes) < n {
		code, err := generateOneRecoveryCode()
		if err != nil {
			return nil, err
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes, nil
}

func generateOneRecoveryCode() (string, error) {
	groups := make([]string, recoveryCodeGroups)
	for g := range groups {
		buf := make([]byte, recoveryCodeGroupLen)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate recovery code: %w", err)
		}
		chars := make([]byte, recoveryCodeGroupLen)
		for i, b := range buf {
			chars[i] = recoveryCodeAlphabet[int(b)%len(recoveryCodeAlphabet)]
		}
		groups[g] = string(chars)
	}
	return strings.Join(groups, "-"), nil
}

// normalizeRecoveryCode makes hashing/comparison tolerant of whitespace and
// case, since users may type or paste codes inconsistently.
func normalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// hashRecoveryCode returns the SHA-256 hex digest of the normalized code.
// Recovery codes are hashed with SHA-256, not argon2id like passwords: they
// carry ~60 bits of server-chosen entropy (not human-chosen), so a slow KDF
// adds cost without adding real offline-guessing resistance, and a login
// checks a candidate against up to ten stored hashes.
func hashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

// consumeRecoveryCode checks candidate against hashes using a constant-time
// comparison per entry. On a match it returns the remaining hashes with the
// matched one removed (so the code cannot be reused) and ok=true.
func consumeRecoveryCode(hashes []string, candidate string) (remaining []string, ok bool) {
	want := hashRecoveryCode(candidate)
	matchedIndex := -1
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(want)) == 1 {
			matchedIndex = i
		}
	}
	if matchedIndex == -1 {
		return hashes, false
	}
	remaining = make([]string, 0, len(hashes)-1)
	remaining = append(remaining, hashes[:matchedIndex]...)
	remaining = append(remaining, hashes[matchedIndex+1:]...)
	return remaining, true
}
