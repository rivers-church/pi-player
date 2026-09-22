package piplayer

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAnsibleConfigShape parses the exact JSON ansible/setup.yml writes onto a
// new device. It is here because the Go side and the playbook drifted apart
// once already: the playbook kept writing {"Mount":{"URL":...}} after MediaDir
// became a plain string, so every freshly provisioned player silently ignored
// its media share.
func TestAnsibleConfigShape(t *testing.T) {
	const written = `{
  "Location": "PiPlayer",
  "MediaDir": "/mnt/networkshare",
  "Debug": true,
  "Login": {},
  "Remote": {
    "Names": ["keyboard"]
  }
}`
	configPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(configPath, "config.json"), []byte(written), 0o600); err != nil {
		t.Fatal(err)
	}

	conf, err := configLoadFromPath(configPath, filepath.Join(t.TempDir(), "fallback"), emptyAssets)
	if err != nil {
		t.Fatalf("loading the provisioned config failed: %v", err)
	}

	if conf.mediaDir() != "/mnt/networkshare" {
		t.Errorf("MediaDir = %q, want the share the playbook configured", conf.mediaDir())
	}
	if conf.locationName() != "PiPlayer" {
		t.Errorf("Location = %q", conf.locationName())
	}
	if !conf.DebugEnabled() {
		t.Error("Debug was not read")
	}
	if len(conf.Remote.Names) != 1 || conf.Remote.Names[0] != "keyboard" {
		t.Errorf("Remote.Names = %v", conf.Remote.Names)
	}
}
