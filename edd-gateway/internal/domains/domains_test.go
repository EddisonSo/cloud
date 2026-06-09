package domains

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"ABC.com":         "abc.com",
		"  Foo.Bar.io  ":  "foo.bar.io",
		"WWW.Example.COM": "www.example.com",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValid(t *testing.T) {
	valid := []string{"abc.com", "www.abc.com", "a-b.example.io", "x.y.z.co.uk"}
	for _, d := range valid {
		if !Valid(d) {
			t.Errorf("Valid(%q) = false, want true", d)
		}
	}
	invalid := []string{"", "nodot", "-abc.com", "abc-.com", "ab..com", "abc.com/path", "*.abc.com", "a b.com"}
	for _, d := range invalid {
		if Valid(d) {
			t.Errorf("Valid(%q) = true, want false", d)
		}
	}
}

func TestGenerateToken(t *testing.T) {
	a := GenerateToken()
	b := GenerateToken()
	if len(a) < 32 {
		t.Errorf("token too short: %q (len %d)", a, len(a))
	}
	if a == b {
		t.Errorf("tokens not unique: %q == %q", a, b)
	}
	for _, c := range a {
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !ok {
			t.Errorf("token has non-[a-z0-9] char: %q", c)
		}
	}
}

func TestVerifyRecordName(t *testing.T) {
	if got := VerifyRecordName("abc.com"); got != "_edd-verify.abc.com" {
		t.Errorf("VerifyRecordName = %q", got)
	}
}

func TestTXTMatches(t *testing.T) {
	token := "deadbeef"
	if !TXTMatches([]string{"other", "deadbeef", "x"}, token) {
		t.Error("expected match")
	}
	if TXTMatches([]string{"other", "x"}, token) {
		t.Error("expected no match")
	}
	if !TXTMatches([]string{"  deadbeef  "}, token) {
		t.Error("expected trimmed match")
	}
}
