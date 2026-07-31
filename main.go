package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/foundry23/switchboard/internal/config"
	"github.com/foundry23/switchboard/internal/spec"
	"github.com/foundry23/switchboard/internal/ui"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	app := &cli.Command{
		Name:      "switchboard",
		Usage:     "the missing control panel for your project; every switch runs a command",
		ArgsUsage: "[spec.json | '<json>' | -]",
		Description: strings.TrimSpace(`
Every project has things that turn on and off: preview environments,
git worktrees, docker services, tunnels. switchboard gathers them
into one interactive checklist — toggle an item and the matching
shell command runs; the box only flips when it succeeds.

EXAMPLES:
   switchboard worktrees.json          spec from a file
   list-previews | switchboard         spec piped from a generator
   switchboard "$(list-previews)"      spec from a command, stdin stays free
   switchboard --config ops/sb.yaml worktrees.json
`),
		Version:               version,
		EnableShellCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "shell", Value: "sh", Sources: cli.EnvVars("SWITCHBOARD_SHELL"), Usage: "shell used to run commands, invoked as '<shell> -c <command>'"},
			&cli.StringFlag{Name: "config", Sources: cli.EnvVars("SWITCHBOARD_CONFIG"), Usage: "repo config YAML defining the switch commands (default: probe " + strings.Join(config.DefaultFiles, ", ") + ")"},
		},
		Action: run,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() > 1 {
		return errors.New(`expected at most one argument: a JSON file, raw JSON, or "-"`)
	}
	cfg, err := config.Load(cmd.String("config"))
	if err != nil {
		return err
	}
	raw, err := readSpec(cmd.Args().First())
	if err != nil {
		return err
	}
	board, err := spec.Parse(raw)
	if err != nil {
		return err
	}
	cfg.Apply(&board)
	if err := board.ExpandArgs(); err != nil {
		return err
	}

	opts := []tea.ProgramOption{tea.WithContext(ctx)}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return fmt.Errorf("stdin is not a terminal and /dev/tty is unavailable: %w", err)
		}
		defer func() { _ = tty.Close() }()
		opts = append(opts, tea.WithInput(tty))
	}

	if _, err := tea.NewProgram(ui.New(ctx, board, cmd.String("shell")), opts...).Run(); err != nil {
		if errors.Is(err, tea.ErrProgramKilled) { // ctx cancelled by a signal
			return nil
		}
		return err
	}
	return nil
}

func readSpec(arg string) ([]byte, error) {
	trimmed := strings.TrimSpace(arg)
	switch {
	case trimmed == "":
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return nil, errors.New("no spec given: pass a JSON file, raw JSON, or pipe JSON on stdin")
		}
		return io.ReadAll(os.Stdin)
	case trimmed == "-":
		return io.ReadAll(os.Stdin)
	case spec.LooksLikeJSON(arg):
		return []byte(arg), nil
	default:
		data, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("reading spec: %w", err)
		}
		return data, nil
	}
}
