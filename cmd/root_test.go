package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestRunRootCmdRejectsAnInvalidLogLevel(t *testing.T) {
	viper.Set("log.level", "verbose")
	t.Cleanup(func() { viper.Set("log.level", nil) })

	err := runRootCmd(rootCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid log level") {
		t.Errorf("runRootCmd error = %v, want an invalid log level error", err)
	}
}
