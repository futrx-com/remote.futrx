package httptransport

import "testing"

func TestApplicationHosts(t *testing.T) {
	const base = "remote.test"
	const id = "abcdef123456"
	for _, tc := range []struct {
		host            string
		reserved, valid bool
	}{
		{id + ".apps." + base, true, true},
		{id + ".apps." + base + ":8443", true, true},
		{"ABCDEF123456.apps." + base, true, true},
		{id + ".apps." + base + ".", true, true},
		{id + ".apps." + base + ".:8443", true, true},
		{"bad.apps." + base, true, false},
		{"apps." + base, true, false},
		{id + ".nested.apps." + base, true, false},
		{id + ".apps." + base + ".evil.test", false, false},
		{id + ".apps.other.test", false, false},
		{base, false, false},
	} {
		got, valid := ApplicationInstanceID(tc.host, base)
		if valid != tc.valid || (valid && got != id) || IsApplicationHost(tc.host, base) != tc.reserved {
			t.Errorf("host %q: id=%q valid=%v", tc.host, got, valid)
		}
	}
	if ApplicationHost(id, base) != id+".apps."+base || ApplicationHost("../bad", base) != "" {
		t.Fatal("incorrect application hostname construction")
	}
}
