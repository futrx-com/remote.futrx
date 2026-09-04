package auth

import (
	"strings"
	"testing"
)

func TestGenerateRecoveryCodesFormatAndUniqueness(t *testing.T) {
	codes, err := generateRecoveryCodes(10)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(codes) != 10 {
		t.Fatalf("len(codes) = %d, want 10", len(codes))
	}
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		parts := strings.Split(code, "-")
		if len(parts) != recoveryCodeGroups {
			t.Fatalf("code %q has %d groups, want %d", code, len(parts), recoveryCodeGroups)
		}
		for _, part := range parts {
			if len(part) != recoveryCodeGroupLen {
				t.Fatalf("code %q group %q has length %d, want %d", code, part, len(part), recoveryCodeGroupLen)
			}
			for _, r := range part {
				if !strings.ContainsRune(recoveryCodeAlphabet, r) {
					t.Fatalf("code %q contains character %q outside the alphabet", code, r)
				}
			}
		}
		if _, dup := seen[code]; dup {
			t.Fatalf("duplicate recovery code generated: %q", code)
		}
		seen[code] = struct{}{}
	}
}

func TestConsumeRecoveryCodeIsOneTimeUse(t *testing.T) {
	codes, err := generateRecoveryCodes(3)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = hashRecoveryCode(c)
	}

	remaining, ok := consumeRecoveryCode(hashes, codes[1])
	if !ok {
		t.Fatal("ConsumeRecoveryCode did not accept a valid code")
	}
	if len(remaining) != len(hashes)-1 {
		t.Fatalf("len(remaining) = %d, want %d", len(remaining), len(hashes)-1)
	}
	for _, h := range remaining {
		if h == hashRecoveryCode(codes[1]) {
			t.Fatal("consumed code hash is still present in remaining")
		}
	}

	if _, ok := consumeRecoveryCode(remaining, codes[1]); ok {
		t.Fatal("consumed code was accepted a second time")
	}

	if _, ok := consumeRecoveryCode(remaining, "not-a-real-code"); ok {
		t.Fatal("garbage code was accepted")
	}

	// Case/whitespace-insensitive: the same code re-typed with different
	// case and surrounding whitespace still matches before consumption.
	remaining2, ok := consumeRecoveryCode(hashes, "  "+strings.ToLower(codes[0])+"  ")
	if !ok {
		t.Fatal("normalized code variant was not accepted")
	}
	if len(remaining2) != len(hashes)-1 {
		t.Fatalf("len(remaining2) = %d, want %d", len(remaining2), len(hashes)-1)
	}
}
