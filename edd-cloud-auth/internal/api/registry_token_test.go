package api

import "testing"

func TestTranslateSAActionToOCI(t *testing.T) {
	cases := map[string]string{
		"pull":   "pull",
		"push":   "push",
		"read":   "pull", // legacy
		"create": "push", // legacy
		"update": "push", // legacy
		"delete": "delete",
	}
	for in, want := range cases {
		if got := translateSAActionToOCI(in); got != want {
			t.Errorf("translateSAActionToOCI(%q) = %q, want %q", in, got, want)
		}
	}
}
