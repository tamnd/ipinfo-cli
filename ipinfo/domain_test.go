package ipinfo

import (
	"testing"
)

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "ipinfo" {
		t.Errorf("Scheme = %q, want ipinfo", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "ipinfo" {
		t.Errorf("Identity.Binary = %q, want ipinfo", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"8.8.8.8", "ip", "8.8.8.8"},
		{"1.1.1.1", "ip", "1.1.1.1"},
		{"2001:db8::1", "ip", "2001:db8::1"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("ip", "8.8.8.8")
	want := "https://ipinfo.io/8.8.8.8"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "8.8.8.8")
	if err == nil {
		t.Error("expected error for unknown type")
	}
}
