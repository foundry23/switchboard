# switchboard

[![CI](https://github.com/foundry23/switchboard/actions/workflows/ci.yml/badge.svg)](https://github.com/foundry23/switchboard/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/foundry23/switchboard)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

switchboard turns a JSON description of switches into an interactive checkbox list.
Flipping a switch runs the `on` or `off` command from your `switchboard.yaml` configuration.

## Install

With [Homebrew](https://brew.sh):

```bash
brew install foundry23/tap/switchboard
```

Or with Go:

```bash
go install github.com/foundry23/switchboard@latest
```

## Usage

```bash
switchboard spec.json           # from a file
some-generator | switchboard    # from stdin (keyboard input falls back to /dev/tty)
switchboard "$(some-generator)" # from a command substitution — stdin stays the keyboard
```

### Arguments

| Flag | Env var | Default | Purpose |
|---|---|---|---|
| `--config` | `SWITCHBOARD_CONFIG` | auto-detected | path to the switch spec YAML |
| `--shell` | `SWITCHBOARD_SHELL` | `sh` | shell every command runs under, as `<shell> -c <command>` |

When `--config` is not given, switchboard looks for `switchboard.yaml`, `switchboard.yml`, `.switchboard.yaml`, or `.switchboard.yml` in the current directory.

### Keys

All keys are case-insensitive.

| Key | Action |
|---|---|
| `↑`/`↓` or `j`/`k` | move between items |
| `space` / `enter` | toggle the selected item |
| `r` | re-check the state of all items |
| `?` | show a legend of every command |
| `q` / `esc` / `ctrl+c` | quit (press twice) |

## Configuration

There are 2 separate specs that are involved in the setup of Switchboard: the list of items, and the switch commands.

The recommended best practice is to store the switch spec in a `switchboard.yaml` file in your project, and to pass the list of items dynamically by piping the list through.

### Item Spec

```json
{
  "title": "Preview environments",
  "items": [
    { "label": "myapp PR-123", "description": "https://pr-123.myapp.localhost", "args": { "ticket": "pr-123" } },
    { "label": "myapp PR-124", "args": { "ticket": "pr-124" } }
  ]
}
```

| Field | Required | Purpose |
|---|---|---|
| `title` | | heading shown above the list |
| `items[].label` | ✓ | text shown next to the checkbox |
| `items[].description` | | dimmed text after the label |
| `items[].args` | | named values for the config's `{{ .name }}` templates |

### Switch Spec

There are currently 3 commands that switchboard handles, and they are all required.

```yaml
switch:
  on: git worktree add ../{{ .ticket }} {{ .ticket }}
  off: git worktree remove ../{{ .ticket }}
  check: git worktree list | grep -q {{ .ticket }}
```

- `switch.on` / `switch.off` — Runs when the item is toggled on or off.
- `switch.check` — Ran on initialisation, and will determine the default state of the switch. Can be re-ran with `r`. `Exit 0` means on and any other exit code means `off`.

## Development

```bash
just build   # go build -o switchboard .
just fmt     # goimports + gofumpt
just check   # go vet + go test
```

`just --list` shows every recipe. CI runs build, vet, race tests, formatting, and linting (golangci-lint).

## Contributing

Contributions are welcome:

1. **Open an issue first** for anything non-trivial so the approach can be agreed before you build it.
2. Fork and branch, and add tests where it makes sense.
3. Run `just check` before opening a PR.
4. Keep commits focused and the CI green.

AI-assisted contributions are welcome, but code **must** be reviewed by a human before being submitted.

Any contributions that have not obviously been reviewed by a human may be closed without comment or justification.

## License

[MIT](LICENSE).
