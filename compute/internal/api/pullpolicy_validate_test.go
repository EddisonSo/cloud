package api

import "testing"

func TestValidPullPolicy(t *testing.T) {
	for _, s := range []string{"Always", "IfNotPresent"} {
		if !validPullPolicy(s) {
			t.Errorf("validPullPolicy(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "always", "ifnotpresent", "Never", "garbage"} {
		if validPullPolicy(s) {
			t.Errorf("validPullPolicy(%q) = true, want false", s)
		}
	}
}
