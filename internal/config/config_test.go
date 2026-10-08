package config

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("TELEGRAM_APITOKEN", "test-token")
	t.Setenv("HOCKEY_BOT_USERNAME", "test_bot")
	for _, path := range []string{"", "/tmp/custom.db"} {
		t.Run(path, func(t *testing.T) {
			t.Setenv("HOCKEY_BOT_DB_PATH", path)
			got := New()
			want := path
			if want == "" {
				want = "./hockey_bot.db"
			}
			if got.TelegramAPIToken != "test-token" || got.BotName != "test_bot" || got.DBPath != want {
				t.Fatalf("wrong config: %+v", got)
			}
		})
	}
}
func TestConfigRejectsMissingRequiredValues(t *testing.T) {
	if os.Getenv("HOCKEY_CONFIG_TEST_CHILD") == "1" {
		New()
		return
	}
	for _, missing := range []string{"TELEGRAM_APITOKEN", "HOCKEY_BOT_USERNAME"} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv("TELEGRAM_APITOKEN", "test-token")
			t.Setenv("HOCKEY_BOT_USERNAME", "test_bot")
			t.Setenv(missing, "")
			t.Setenv("HOCKEY_CONFIG_TEST_CHILD", "1")
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestConfigRejectsMissingRequiredValues$")
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(out), missing+" environment variable required") {
				t.Fatalf("missing %s was not rejected: %s (%v)", missing, out, err)
			}
		})
	}
}
