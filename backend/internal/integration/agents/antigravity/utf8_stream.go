package antigravity

import (
	"unicode/utf8"
)

// UTF8StreamDecoder incrementally decodes a stream of UTF-8 byte chunks into strings.
// It preserves incomplete multibyte rune sequences between arbitrary chunk boundaries,
// preventing U+FFFD (Unicode replacement character) corruption caused by converting
// partial UTF-8 byte sequences independently.
type UTF8StreamDecoder struct {
	pending []byte
}

// Decode processes a chunk of raw bytes, buffering any incomplete trailing multibyte
// UTF-8 sequence, and returns the valid decoded UTF-8 string prefix.
func (d *UTF8StreamDecoder) Decode(chunk []byte) string {
	if len(chunk) == 0 {
		return ""
	}
	var buf []byte
	if len(d.pending) > 0 {
		buf = append(d.pending, chunk...)
		d.pending = nil
	} else {
		buf = chunk
	}

	if len(buf) == 0 {
		return ""
	}

	// Check if the trailing bytes in buf form an incomplete UTF-8 rune.
	// In UTF-8, a code point is at most utf8.UTFMax (4 bytes).
	// An incomplete sequence at the end can be at most min(len(buf), utf8.UTFMax-1) bytes.
	incompleteLen := 0
	maxCheck := utf8.UTFMax - 1
	if len(buf) < maxCheck {
		maxCheck = len(buf)
	}

	for i := 1; i <= maxCheck; i++ {
		tail := buf[len(buf)-i:]
		if utf8.RuneStart(tail[0]) {
			if !utf8.FullRune(tail) {
				incompleteLen = i
			}
			break
		}
	}

	if incompleteLen > 0 {
		split := len(buf) - incompleteLen
		d.pending = append([]byte(nil), buf[split:]...)
		buf = buf[:split]
	}

	if len(buf) == 0 {
		return ""
	}
	return string(buf)
}

// Flush decodes and returns any remaining pending bytes at the end of the stream.
func (d *UTF8StreamDecoder) Flush() string {
	if len(d.pending) == 0 {
		return ""
	}
	s := string(d.pending)
	d.pending = nil
	return s
}
