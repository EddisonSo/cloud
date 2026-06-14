package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestResolvePullPolicy(t *testing.T) {
	cases := map[string]corev1.PullPolicy{
		"Always":       corev1.PullAlways,
		"IfNotPresent": corev1.PullIfNotPresent,
		"":             corev1.PullIfNotPresent, // default
		"garbage":      corev1.PullIfNotPresent, // unknown → safe default
	}
	for in, want := range cases {
		if got := resolvePullPolicy(in); got != want {
			t.Errorf("resolvePullPolicy(%q) = %v, want %v", in, got, want)
		}
	}
}
