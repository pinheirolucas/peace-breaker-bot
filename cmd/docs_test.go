package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"go.yaml.in/yaml/v3"
)

func settingFlags(t *testing.T) []*pflag.Flag {
	t.Helper()

	var flags []*pflag.Flag
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "config" {
			flags = append(flags, f)
		}
	})

	if len(flags) == 0 {
		t.Fatal("rootCmd has no setting flags")
	}

	return flags
}

func configKey(flag string) string {
	return strings.Replace(flag, "-", ".", 1)
}

func envVar(flag string) string {
	return "PBB_" + strings.ToUpper(strings.ReplaceAll(flag, "-", "_"))
}

func TestSampleConfigCoversEveryFlag(t *testing.T) {
	raw, err := os.ReadFile("../.peace-breaker-bot.sample.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var sample map[string]map[string]interface{}
	if err := yaml.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("sample config does not parse: %v", err)
	}

	for _, f := range settingFlags(t) {
		section, key, _ := strings.Cut(configKey(f.Name), ".")
		if _, ok := sample[section][key]; !ok {
			t.Errorf("sample config has no %s.%s for --%s", section, key, f.Name)
		}
		for _, want := range []string{"--" + f.Name, envVar(f.Name)} {
			if !strings.Contains(string(raw), want) {
				t.Errorf("sample config doesn't mention %s", want)
			}
		}
	}
}

func TestReadmeTableCoversEveryFlag(t *testing.T) {
	raw, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range settingFlags(t) {
		row := "| `" + configKey(f.Name) + "` | `--" + f.Name + "` | `" + envVar(f.Name) + "` |"
		if !strings.Contains(string(raw), row) {
			t.Errorf("README settings table has no row containing %s", row)
		}
	}
}
