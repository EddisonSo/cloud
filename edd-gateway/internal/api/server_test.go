package api

import "testing"

func TestAllowedPort(t *testing.T) {
	cases := []struct {
		port int
		want bool
	}{
		{80, true},
		{443, true},
		{8000, true},
		{8999, true},
		{7999, false},
		{9000, false},
		{22, false},
		{0, false},
	}
	for _, c := range cases {
		if got := allowedPort(c.port); got != c.want {
			t.Errorf("allowedPort(%d) = %v, want %v", c.port, got, c.want)
		}
	}
}
