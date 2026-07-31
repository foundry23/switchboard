package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/foundry23/switchboard/internal/spec"
)

type Switch struct {
	On    string `json:"on"`
	Off   string `json:"off"`
	Check string `json:"check"`
}

func (c *Switch) UnmarshalJSON(data []byte) error {
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, run := range raw {
		switch key {
		case "on", "true":
			c.On = run
		case "off", "false":
			c.Off = run
		case "check":
			c.Check = run
		default:
			return fmt.Errorf(`unknown command %q: switch defines "on", "off", and "check"`, key)
		}
	}
	return nil
}

type Config struct {
	Switch Switch `json:"switch"`
}

var DefaultFiles = []string{"switchboard.yaml", "switchboard.yml", ".switchboard.yaml", ".switchboard.yml"}

func Load(path string) (Config, error) {
	if path == "" {
		found, ok := probe()
		if !ok {
			return Config{}, fmt.Errorf(
				`no config found (looked for %s): switchboard needs a config whose "switch" section defines the "on", "off", and "check" commands`,
				strings.Join(DefaultFiles, ", "))
		}
		path = found
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func probe() (string, bool) {
	for _, name := range DefaultFiles {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			return name, true
		}
	}
	return "", false
}

func (c Config) validate() error {
	var missing []string
	for _, command := range []struct{ name, run string }{
		{"on", c.Switch.On}, {"off", c.Switch.Off}, {"check", c.Switch.Check},
	} {
		if strings.TrimSpace(command.run) == "" {
			missing = append(missing, fmt.Sprintf("%q", command.name))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			`switch is missing %s — it must define "on" and "off" (what a flip runs) and "check" (how to probe an item's real state; exit 0 = on)`,
			strings.Join(missing, ", "))
	}

	return nil
}

func (c Config) Apply(board *spec.Board) {
	for i := range board.Items {
		item := &board.Items[i]
		item.On = c.Switch.On
		item.Off = c.Switch.Off
		item.Check = c.Switch.Check
	}
}
