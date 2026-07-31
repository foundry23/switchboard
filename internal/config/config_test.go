package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/foundry23/switchboard/internal/spec"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const switchStub = "switch:\n  on: up\n  off: down\n  check: probe\n"

func TestLoadValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "switchboard.yaml")
	write(t, path, `
switch:
  on: preview up {{ .ticket }}
  off: preview down {{ .ticket }}
  check: preview status {{ .ticket }}
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Unquoted on/off map keys arrive as YAML booleans and must be mapped back.
	if cfg.Switch.On != "preview up {{ .ticket }}" || cfg.Switch.Off != "preview down {{ .ticket }}" {
		t.Errorf("switch = %+v", cfg.Switch)
	}
	if cfg.Switch.Check != "preview status {{ .ticket }}" {
		t.Errorf("check = %q", cfg.Switch.Check)
	}
}

func TestLoadUnknownCommandRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "switchboard.yaml")
	write(t, path, "switch:\n  on: a\n  off: b\n  check: c\n  sideways: d\n")
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), `unknown command "sideways"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadErrorsWithoutConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := Load("")
	if err == nil || !strings.Contains(err.Error(), "no config found") {
		t.Fatalf("err = %v, want a missing-config error", err)
	}
	if !strings.Contains(err.Error(), "switchboard.yaml") {
		t.Errorf("err = %v, should say which files were probed", err)
	}
}

func TestLoadProbesDefaultFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, ".switchboard.yml", switchStub)
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Switch.On != "up" {
		t.Errorf("cfg = %+v, want probed commands", cfg)
	}
}

func TestLoadExplicitMissingPathErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected an error for an explicit missing path")
	}
}

func TestLoadValidationErrors(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty file", "", `switch is missing "on", "off", "check"`},
		{"missing check", "switch:\n  on: a\n  off: b\n", `switch is missing "check"`},
		{"missing off and check", "switch:\n  on: a\n", `switch is missing "off", "check"`},
		{"blank command", "switch:\n  on: a\n  off: b\n  check: \" \"\n", `switch is missing "check"`},
		{"invalid yaml", "commands: [", "parsing"},
		{"switch as list is invalid", "switch:\n  - toggle: on\n    run: a\n", "parsing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "switchboard.yaml")
			write(t, path, tc.in)
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestApplyAttachesCommandsToEveryItem(t *testing.T) {
	cfg := Config{Switch: Switch{On: "config-on", Off: "config-off", Check: "config-check"}}
	board := spec.Board{Items: []spec.Item{{Label: "a"}, {Label: "b"}}}
	cfg.Apply(&board)

	for i := range board.Items {
		it := board.Items[i]
		if it.On != "config-on" || it.Off != "config-off" || it.Check != "config-check" {
			t.Errorf("items[%d] = %+v, want config commands attached", i, it)
		}
	}
}
