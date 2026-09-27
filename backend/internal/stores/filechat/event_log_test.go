package filechat

import "testing"

func TestParseBackend(t *testing.T) {
	cases := []struct {
		raw     string
		want    Backend
		wantErr bool
	}{
		{raw: "", want: BackendSQLite},
		{raw: "jsonl", want: BackendJSONL},
		{raw: "JSONL", want: BackendJSONL},
		{raw: " sqlite ", want: BackendSQLite},
		{raw: "mysql", wantErr: true},
		{raw: "SQLITE3", wantErr: true},
	}
	for _, test := range cases {
		got, err := ParseBackend(test.raw)
		if test.wantErr {
			if err == nil {
				t.Fatalf("ParseBackend(%q) = %q, want an error", test.raw, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseBackend(%q): %v", test.raw, err)
		}
		if got != test.want {
			t.Fatalf("ParseBackend(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}
