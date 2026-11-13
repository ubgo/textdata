package githubplugin

import (
	"os"
	"testing"
)

// TestMain clears the token variables gh reads before any test runs. The
// account label ("via GH_TOKEN" or "stored gh auth") depends on them, so an
// ambient token (a release tool exports one) would change what the tests
// see. Most tests run in parallel, which t.Setenv forbids, so the variables
// are cleared once for the whole binary; tests that need a token set it with
// t.Setenv themselves.
func TestMain(m *testing.M) {
	for _, v := range tokenEnvVars {
		_ = os.Unsetenv(v)
	}
	os.Exit(m.Run())
}
