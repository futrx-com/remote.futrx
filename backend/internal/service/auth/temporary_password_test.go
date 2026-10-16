package auth

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"
	"testing/iotest"
)

var temporaryPasswordPattern = regexp.MustCompile(`^[2-9A-HJKMNP-Z]{5}(-[2-9A-HJKMNP-Z]{5}){3}$`)

func TestTemporaryPasswordAlphabetHas31UnambiguousSymbols(t *testing.T) {
	if len(temporaryPasswordAlphabet) != 31 {
		t.Fatalf("alphabet has %d symbols, want 31", len(temporaryPasswordAlphabet))
	}
	seen := map[rune]bool{}
	for _, r := range temporaryPasswordAlphabet {
		if seen[r] {
			t.Fatalf("duplicate symbol %q", r)
		}
		seen[r] = true
	}
	for _, lookalike := range "01ILOilo" {
		if seen[lookalike] {
			t.Fatalf("alphabet contains look-alike %q", lookalike)
		}
	}
	if temporaryPasswordAcceptBelow != 248 {
		t.Fatalf("accept threshold = %d, want 248 (largest multiple of 31 below 256)", temporaryPasswordAcceptBelow)
	}
}

func TestGenerateTemporaryPasswordFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := range 500 {
		password, err := GenerateTemporaryPassword()
		if err != nil {
			t.Fatal(err)
		}
		if !temporaryPasswordPattern.MatchString(password) {
			t.Fatalf("password %q does not match XXXXX-XXXXX-XXXXX-XXXXX over the alphabet", password)
		}
		if len(password) != 23 {
			t.Fatalf("len = %d, want 23", len(password))
		}
		if seen[password] {
			t.Fatalf("duplicate password %q in 500 draws", password)
		}
		seen[password] = true
		// It must be usable as a real password: a temporary password the
		// hasher refuses would make an invitation impossible.
		if i < 3 {
			if _, err := HashPassword(password); err != nil {
				t.Fatalf("HashPassword(%q): %v", password, err)
			}
		}
	}
}

func TestGenerateTemporaryPasswordMapsBytesInOrder(t *testing.T) {
	// Bytes 0..30 are the alphabet in order; 31..61 wrap round once more.
	input := make([]byte, 0, 20)
	for i := range 20 {
		input = append(input, byte(i))
	}
	got, err := generateTemporaryPassword(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	for i := range 20 {
		if i > 0 && i%5 == 0 {
			want.WriteByte('-')
		}
		want.WriteByte(temporaryPasswordAlphabet[i])
	}
	if got != want.String() {
		t.Fatalf("got %q, want %q", got, want.String())
	}
}

func TestGenerateTemporaryPasswordRejectsBiasedBytes(t *testing.T) {
	// 248..255 would favour the first 8 symbols if reduced modulo 31, so they
	// must be skipped, including a long run of them and the boundary 247.
	input := append(bytes.Repeat([]byte{255, 250, 248}, 40), 247) // 247 % 31 == 30
	input = append(input, bytes.Repeat([]byte{0}, 19)...)
	got, err := generateTemporaryPassword(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := "Z" + strings.Repeat("2", 19)
	want = want[:5] + "-" + want[5:10] + "-" + want[10:15] + "-" + want[15:]
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGenerateTemporaryPasswordUniformOverEveryAcceptedByte(t *testing.T) {
	counts := map[byte]int{}
	for b := 0; b < 256; b++ {
		got, err := generateTemporaryPassword(bytes.NewReader(append([]byte{byte(b)}, bytes.Repeat([]byte{0}, 19)...)))
		if int(b) >= temporaryPasswordAcceptBelow {
			// The rejected byte is dropped, so the stream runs dry.
			if err == nil {
				t.Fatalf("byte %d was accepted (%q)", b, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("byte %d: %v", b, err)
		}
		counts[got[0]]++
	}
	for _, symbol := range []byte(temporaryPasswordAlphabet) {
		if counts[symbol] != 8 {
			t.Fatalf("symbol %q reachable from %d bytes, want exactly 8", symbol, counts[symbol])
		}
	}
}

func TestGenerateTemporaryPasswordSourceFailures(t *testing.T) {
	for name, source := range map[string]func() error{
		"read error": func() error {
			_, err := generateTemporaryPassword(iotest.ErrReader(errors.New("entropy unavailable")))
			return err
		},
		"exhausted": func() error {
			_, err := generateTemporaryPassword(bytes.NewReader([]byte{1, 2, 3}))
			return err
		},
	} {
		if err := source(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
