package envutil

import "regexp"

var envVarNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func LooksLikeEnvVar(s string) bool {
	return envVarNameRe.MatchString(s)
}
