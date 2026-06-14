package api

// validPullPolicy reports whether s is an accepted image pull policy.
func validPullPolicy(s string) bool {
	return s == "Always" || s == "IfNotPresent"
}
