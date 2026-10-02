package ui

import (
	"os"
	"testing"
)

// TestMain points the config directory at a scratch location so no test can
// read or write the developer's real settings, themes or event log.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rigwatch-ui-test")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
