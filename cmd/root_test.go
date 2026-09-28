package cmd

import (
	"os"
	"path/filepath"
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

func TestDataDirDefaultsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := dataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".peace-breaker-bot"); got != want {
		t.Errorf("dataDir() = %q, want %q", got, want)
	}
}

func TestDataDirOrCreateUsesTheSetting(t *testing.T) {
	want := filepath.Join(t.TempDir(), "nested", "data")
	viper.Set("data.dir", "  "+want+"  ")
	t.Cleanup(func() { viper.Set("data.dir", nil) })

	got, err := dataDirOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("dataDirOrCreate() = %q, want %q", got, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Errorf("data dir not created: %v", err)
	}
}

func TestRunRootCmdRejectsAnOwnerThatIsNotADiscordUsername(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	viper.Set("bot.token", "token")
	viper.Set("bot.owner", "Name#1234")
	t.Cleanup(func() {
		viper.Set("bot.token", nil)
		viper.Set("bot.owner", nil)
	})

	err := runRootCmd(rootCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid bot owner") {
		t.Errorf("runRootCmd error = %v, want an invalid bot owner error", err)
	}
}
