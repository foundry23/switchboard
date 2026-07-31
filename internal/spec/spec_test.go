package spec

import (
	"strings"
	"testing"

	"github.com/foundry23/switchboard/internal/runner"
)

func TestParseBoard(t *testing.T) {
	board, err := Parse([]byte(`{
		"title": "Preview environments",
		"items": [
			{"label": "pr-123", "description": "myapp", "checked": true, "args": {"ticket": "pr-123"}},
			{"label": "pr-124"}
		]
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if board.Title != "Preview environments" {
		t.Errorf("title = %q", board.Title)
	}
	if len(board.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(board.Items))
	}
	first := board.Items[0]
	if first.Label != "pr-123" || first.Description != "myapp" {
		t.Errorf("first item = %+v", first)
	}
	if first.Args["ticket"] != "pr-123" {
		t.Errorf("args = %+v", first.Args)
	}
	// "checked" in the spec is an unknown field: state only ever comes from
	// the check command.
	if first.Checked || board.Items[1].Checked {
		t.Error("a spec must not be able to declare an item checked")
	}
}

func TestParseBareArrayShorthand(t *testing.T) {
	board, err := Parse([]byte(`  [{"label": "a"}, {"label": "b"}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(board.Items) != 2 || board.Items[0].Label != "a" {
		t.Errorf("items = %+v", board.Items)
	}
}

func TestParseUnknownFieldsTolerated(t *testing.T) {
	board, err := Parse([]byte(`{"items": [{"label": "a", "id": "x", "extra": 1, "on": "not a command"}]}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if board.Items[0].On != "" {
		t.Error("spec fields must never populate commands — those come from the config")
	}
}

func TestParseNamedArgs(t *testing.T) {
	board, err := Parse([]byte(`[{"label": "a", "args": {"ticket": "ENG-1", "env": "dev"}}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	args := board.Items[0].Args
	if args["ticket"] != "ENG-1" || args["env"] != "dev" {
		t.Errorf("args = %v", args)
	}
}

func TestParseArgsMustBeNamed(t *testing.T) {
	for _, in := range []string{
		`[{"label": "a", "args": "nope"}]`,
		`[{"label": "a", "args": ["positional", "is", "gone"]}]`,
	} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("Parse(%s) should reject non-object args", in)
		}
	}
}

func TestExpandArgsRendersNamedTemplates(t *testing.T) {
	board, err := Parse([]byte(`[{"label": "a", "args": {"ticket": "ENG-1", "env": "dev"}}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	board.Items[0].On = "deploy {{ .ticket }} to {{ .env }}"
	board.Items[0].Off = "teardown {{ .ticket }}"
	if err := board.ExpandArgs(); err != nil {
		t.Fatalf("ExpandArgs: %v", err)
	}
	if got := board.Items[0].On; got != "deploy ENG-1 to dev" {
		t.Errorf("on = %q", got)
	}
	if got := board.Items[0].Off; got != "teardown ENG-1" {
		t.Errorf("off = %q", got)
	}
}

func TestExpandArgsQuotesUnsafeValues(t *testing.T) {
	board, err := Parse([]byte(`[{"label": "a", "args": {"msg": "it's $(boom) here"}}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	board.Items[0].On = "echo {{ .msg }} and 'quoted {{ .msg }}'"
	if err := board.ExpandArgs(); err != nil {
		t.Fatalf("ExpandArgs: %v", err)
	}
	want := `echo 'it'"'"'s $(boom) here' and 'quoted 'it'"'"'s $(boom) here''`
	if got := board.Items[0].On; got != want {
		t.Errorf("on = %q, want %q", got, want)
	}
}

func TestExpandedCommandsAreInjectionSafe(t *testing.T) {
	hostile := `a b; $(echo injected) 'x' ~`
	board, err := Parse([]byte(`[{"label": "a"}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	board.Items[0].On = "printf '%s' {{ .v }}"
	board.Items[0].Args = map[string]string{"v": hostile}
	if err := board.ExpandArgs(); err != nil {
		t.Fatalf("ExpandArgs: %v", err)
	}
	out, err := runner.Run(t.Context(), "sh", board.Items[0].On, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != hostile {
		t.Errorf("output = %q, want the hostile value passed through literally", out)
	}
}

func TestExpandArgsUnknownNameErrors(t *testing.T) {
	board, err := Parse([]byte(`[{"label": "a", "args": {"ticket": "ENG-1"}}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	board.Items[0].On = "deploy {{ .tikcet }}"
	err = board.ExpandArgs()
	if err == nil || !strings.Contains(err.Error(), "items[0] (a): on:") {
		t.Fatalf("err = %v", err)
	}
}

func TestLooksLikeJSON(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`{"items": []}`, true},
		{`  [{"label": "a"}]`, true},
		{"\n\t{", true},
		{"spec.json", false},
		{"-", false},
		{"", false},
		{"42", false},
	}
	for _, tc := range cases {
		if got := LooksLikeJSON(tc.in); got != tc.want {
			t.Errorf("LooksLikeJSON(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"invalid json", `{`, "parsing spec"},
		{"no items", `{"title": "t"}`, "no items"},
		{"empty items", `[]`, "no items"},
		{"missing label", `[{"label": "ok"}, {"description": "no label"}]`, "items[1]: label is required"},
		{"blank label", `[{"label": "  "}]`, "items[0]: label is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}
