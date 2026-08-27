package webpush

import "testing"

func TestNormalizeSubscriber(t *testing.T) {
	for _, test := range []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "https://remote.example.com/", want: "https://remote.example.com"},
		{input: "mailto:ops@example.com", want: "ops@example.com"},
		{input: "ops@example.com", want: "ops@example.com"},
		{input: "", wantErr: true},
		{input: "http://remote.example.com", wantErr: true},
		{input: "https://user@remote.example.com", wantErr: true},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, err := normalizeSubscriber(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeSubscriber(%q) = %q, want an error", test.input, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("normalizeSubscriber(%q) = %q, %v; want %q", test.input, got, err, test.want)
			}
		})
	}
}
