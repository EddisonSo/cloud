package k8s

import corev1 "k8s.io/api/core/v1"

// resolvePullPolicy maps a stored pull_policy string to a Kubernetes pull policy.
// Anything other than "Always" resolves to IfNotPresent (the safe default that
// preserves prior behavior).
func resolvePullPolicy(s string) corev1.PullPolicy {
	if s == "Always" {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
}
