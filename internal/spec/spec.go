package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"unicode"

	"al.essio.dev/pkg/shellescape"
)

type Item struct {
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Args        map[string]string `json:"args,omitempty"` // named values for {{ .name }} templates

	// Computed state from the config's `check` switch command and state after toggle switches ran
	Checked bool `json:"-"`

	// Switch commands from spec
	On    string `json:"-"`
	Off   string `json:"-"`
	Check string `json:"-"`
}

// Board is a titled list of switches.
type Board struct {
	Title string `json:"title,omitempty"`
	Items []Item `json:"items"`
}

func (b *Board) UnmarshalJSON(data []byte) error {
	if firstByte(data) == '[' {
		return json.Unmarshal(data, &b.Items)
	}
	type plain Board
	return json.Unmarshal(data, (*plain)(b))
}

func Parse(data []byte) (Board, error) {
	var board Board
	if err := json.Unmarshal(data, &board); err != nil {
		return Board{}, fmt.Errorf("parsing spec: %w", err)
	}

	if len(board.Items) == 0 {
		return Board{}, errors.New("spec has no items")
	}
	for i, item := range board.Items {
		if strings.TrimSpace(item.Label) == "" {
			return Board{}, fmt.Errorf("items[%d]: label is required", i)
		}
	}
	return board, nil
}

func (b *Board) ExpandArgs() error {
	for i := range b.Items {
		item := &b.Items[i]
		if len(item.Args) == 0 {
			continue
		}
		params := make(map[string]string, len(item.Args))
		for name, value := range item.Args {
			params[name] = shellescape.Quote(value)
		}
		var err error
		if item.On, err = renderCommand(item.On, params); err != nil {
			return fmt.Errorf("items[%d] (%s): on: %w", i, item.Label, err)
		}
		if item.Off, err = renderCommand(item.Off, params); err != nil {
			return fmt.Errorf("items[%d] (%s): off: %w", i, item.Label, err)
		}
		if item.Check, err = renderCommand(item.Check, params); err != nil {
			return fmt.Errorf("items[%d] (%s): check: %w", i, item.Label, err)
		}
	}
	return nil
}

func renderCommand(command string, params map[string]string) (string, error) {
	if command == "" {
		return "", nil
	}
	t, err := template.New("command").Option("missingkey=error").Parse(command)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func LooksLikeJSON(s string) bool {
	b := firstByte([]byte(s))
	return b == '{' || b == '['
}

func firstByte(data []byte) byte {
	for _, b := range data {
		if !unicode.IsSpace(rune(b)) {
			return b
		}
	}
	return 0
}
