package antigravity

import (
	"strings"
	"testing"
)

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func TestUTF8StreamDecoderChunkBoundaries(t *testing.T) {
	testCases := []string{
		"مرحبا بالعالم",
		"إدارة المستخدمين والصلاحيات",
		"يمكن استخدام Angular مع Python و FastAPI.",
		"نظام تسجيل الدخول باستخدام OAuth 2.0 / OpenID Connect (OIDC) مناسب للشركة",
		"**Angular** هو إطار عمل للواجهة الأمامية **(Frontend / Client-side UI)**.",
		"يتم بناء الواجهة باستخدام Angular لتجربة Single Page Application - SPA سريعة.",
		"هذا اختبار عربي للتأكد من أن ترميز UTF-8 يعمل بشكل صحيح.",
		"🚀 Testing multibyte unicode characters with emojis 🎉 and symbols ©®™",
	}

	for _, text := range testCases {
		rawBytes := []byte(text)

		// Test 1: 1-byte chunks (worst case chunk boundary splitting every multibyte sequence)
		t.Run("1-byte chunks: "+truncateStr(text, 20), func(t *testing.T) {
			var decoder UTF8StreamDecoder
			var sb strings.Builder
			for i := 0; i < len(rawBytes); i++ {
				out := decoder.Decode(rawBytes[i : i+1])
				sb.WriteString(out)
			}
			sb.WriteString(decoder.Flush())

			result := sb.String()
			if result != text {
				t.Fatalf("expected %q, got %q", text, result)
			}
			if strings.ContainsRune(result, '\uFFFD') {
				t.Fatalf("result contains U+FFFD: %q", result)
			}
		})

		// Test 2: Split at every single possible 2-way byte split point [0:split] and [split:len]
		t.Run("All 2-way splits: "+truncateStr(text, 20), func(t *testing.T) {
			for split := 1; split < len(rawBytes); split++ {
				var decoder UTF8StreamDecoder
				var sb strings.Builder
				sb.WriteString(decoder.Decode(rawBytes[:split]))
				sb.WriteString(decoder.Decode(rawBytes[split:]))
				sb.WriteString(decoder.Flush())

				result := sb.String()
				if result != text {
					t.Fatalf("split at %d: expected %q, got %q", split, text, result)
				}
				if strings.ContainsRune(result, '\uFFFD') {
					t.Fatalf("split at %d: result contains U+FFFD: %q", split, result)
				}
			}
		})

		// Test 3: Various chunk sizes from 1 to 16 bytes
		for chunkSize := 1; chunkSize <= 16; chunkSize++ {
			var decoder UTF8StreamDecoder
			var sb strings.Builder
			for offset := 0; offset < len(rawBytes); offset += chunkSize {
				end := offset + chunkSize
				if end > len(rawBytes) {
					end = len(rawBytes)
				}
				sb.WriteString(decoder.Decode(rawBytes[offset:end]))
			}
			sb.WriteString(decoder.Flush())

			result := sb.String()
			if result != text {
				t.Fatalf("chunkSize %d: expected %q, got %q", chunkSize, text, result)
			}
			if strings.ContainsRune(result, '\uFFFD') {
				t.Fatalf("chunkSize %d: result contains U+FFFD: %q", chunkSize, result)
			}
		}
	}
}
