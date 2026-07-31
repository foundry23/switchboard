package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/foundry23/switchboard/internal/runner"
	"github.com/foundry23/switchboard/internal/spec"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	checkedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	busyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	dimStyle     = lipgloss.NewStyle().Faint(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	keyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// How many lines of a command's output to show after an error. The rest is truncated.
const outputTailLines = 3

// How long to show the quit prompt for confirmation.
const quitConfirmWindow = 2 * time.Second

// How long to show a success note after a command succeeds.
const noteTTL = 3 * time.Second

type item struct {
	spec.Item
	busy      bool
	running   string       // "checking" during a probe, "" for toggles
	live      *runner.Live // streamed output of the currently running command
	startedAt time.Time
	errMsg    string
	okMsg     string
	noteSeq   int // invalidates stale clearNoteMsg timers
	output    string
}

type resultMsg struct {
	index  int
	target bool // the state a toggle was trying to reach (checked or unchecked)
	check  bool // set instead when a state probe ran; err nil = on
	output string
	err    error
}

type quitTimeoutMsg struct{ seq int }

type clearNoteMsg struct{ index, seq int }

type Model struct {
	ctx      context.Context
	cancel   context.CancelFunc
	title    string
	shell    string
	items    []item
	cursor   int
	width    int
	spin     spinner.Model
	busy     int
	showHelp bool

	quitArmed bool
	quitKey   string
	quitSeq   int
}

func New(ctx context.Context, board spec.Board, shell string) Model {
	ctx, cancel := context.WithCancel(ctx)
	items := make([]item, len(board.Items))
	for i, it := range board.Items {
		items[i] = item{Item: it}
	}
	m := Model{
		ctx:    ctx,
		cancel: cancel,
		title:  board.Title,
		shell:  shell,
		items:  items,
		spin:   spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(busyStyle)),
	}
	for i := range m.items {
		if m.items[i].Check != "" {
			m.markChecking(i)
		}
	}
	return m
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.items {
		if m.items[i].busy {
			cmds = append(cmds, m.checkCmd(i))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(append(cmds, m.spin.Tick)...)
}

func (m *Model) markChecking(i int) {
	it := &m.items[i]
	it.busy = true
	it.running = "checking"
	it.errMsg, it.okMsg, it.output = "", "", ""
	it.live = &runner.Live{}
	it.startedAt = time.Now()
	m.busy++
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			var (
				model tea.Model = m
				cmds  []tea.Cmd
			)
			for _, r := range msg.Runes {
				var cmd tea.Cmd
				model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			return model, tea.Batch(cmds...)
		}
		key := strings.ToLower(msg.String()) // all key handling is case-insensitive
		if key != "q" && key != "esc" && key != "ctrl+c" {
			m.quitArmed = false
		}
		switch key {
		case "q", "esc", "ctrl+c":
			if m.quitArmed {
				return m.quit()
			}
			m.quitArmed = true
			m.quitKey = key
			m.quitSeq++
			seq := m.quitSeq
			return m, tea.Tick(quitConfirmWindow, func(time.Time) tea.Msg { return quitTimeoutMsg{seq: seq} })
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ", "enter":
			return m.toggle(m.cursor)
		case "r":
			return m.refresh()
		case "?":
			m.showHelp = !m.showHelp
		}

	case resultMsg:
		it := &m.items[msg.index]
		it.busy = false
		it.running = ""
		m.busy--
		var took time.Duration
		if !it.startedAt.IsZero() {
			took = time.Since(it.startedAt)
		}
		it.live = nil
		switch {
		case msg.check:
			// A probe's exit code IS the state — non-zero means off, not failure.
			it.Checked = msg.err == nil
		case msg.err != nil:
			it.errMsg = fmt.Sprintf("failed: %v", msg.err)
			it.output = msg.output
		default:
			it.Checked = msg.target
			if text := lastLine(msg.output); text != "" {
				it.okMsg = fmt.Sprintf("✓ %s · %s", text, fmtDuration(took))
			} else {
				it.okMsg = "✓ " + fmtDuration(took)
			}
			it.errMsg, it.output = "", ""
			it.noteSeq++
			index, seq := msg.index, it.noteSeq
			return m, tea.Tick(noteTTL, func(time.Time) tea.Msg { return clearNoteMsg{index: index, seq: seq} })
		}

	case clearNoteMsg:
		it := &m.items[msg.index]
		if it.noteSeq == msg.seq {
			it.okMsg = ""
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width

	case quitTimeoutMsg:
		if msg.seq == m.quitSeq {
			m.quitArmed = false
		}

	case spinner.TickMsg:
		if m.busy > 0 {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) toggle(i int) (tea.Model, tea.Cmd) {
	it := &m.items[i]
	if it.busy {
		return m, nil
	}
	target := !it.Checked
	command := it.On
	if !target {
		command = it.Off
	}
	it.errMsg, it.okMsg, it.output = "", "", ""
	it.busy = true
	it.running = ""
	it.live = &runner.Live{}
	it.startedAt = time.Now()
	m.busy++
	run := m.runCmd(i, target, command, it.live)
	if m.busy == 1 {
		return m, tea.Batch(run, m.spin.Tick)
	}
	return m, run
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	wasBusy := m.busy
	var cmds []tea.Cmd
	for i := range m.items {
		if m.items[i].Check == "" || m.items[i].busy {
			continue
		}
		m.markChecking(i)
		cmds = append(cmds, m.checkCmd(i))
	}
	if len(cmds) == 0 {
		return m, nil
	}
	if wasBusy == 0 {
		cmds = append(cmds, m.spin.Tick)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) checkCmd(index int) tea.Cmd {
	ctx, shell := m.ctx, m.shell
	command, live := m.items[index].Check, m.items[index].live
	return func() tea.Msg {
		out, err := runner.Run(ctx, shell, command, live)
		return resultMsg{index: index, check: true, output: out, err: err}
	}
}

func (m Model) runCmd(index int, target bool, command string, live *runner.Live) tea.Cmd {
	ctx, shell := m.ctx, m.shell
	return func() tea.Msg {
		out, err := runner.Run(ctx, shell, command, live)
		return resultMsg{index: index, target: target, output: out, err: err}
	}
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	return m, tea.Quit
}

func (m Model) View() string {
	var b strings.Builder
	if m.title != "" {
		b.WriteString(titleStyle.Render(m.title) + "\n\n")
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	for i, it := range m.items {
		prefix := "  "
		if i == m.cursor {
			prefix = cursorStyle.Render("❯ ")
		}
		box := "[ ]"
		switch {
		case it.busy:
			box = "[" + m.spin.View() + "]"
		case it.Checked:
			box = checkedStyle.Render("[x]")
		}
		label := it.Label
		if i == m.cursor {
			label = titleStyle.Render(label)
		}
		base := prefix + box + " " + label

		var right string
		var rightStyle lipgloss.Style
		switch {
		case it.busy:
			text := ""
			if it.live != nil {
				text = it.live.LastLine()
			}
			if text == "" {
				text = it.running // key-bound command name
			}
			if text == "" {
				text = "running"
			}
			right = fmt.Sprintf("%s · %s", text, time.Since(it.startedAt).Truncate(time.Second))
			rightStyle = busyStyle
		case it.errMsg != "":
			right = "✗ " + it.errMsg
			rightStyle = errorStyle
		case it.okMsg != "":
			right = it.okMsg
			rightStyle = checkedStyle
		}

		b.WriteString(renderRow(width, base, it.Description, right, rightStyle) + "\n")
		if it.errMsg != "" {
			for _, l := range tail(it.output, outputTailLines) {
				b.WriteString(dimStyle.Render("        "+l) + "\n")
			}
		}
	}

	if m.showHelp {
		b.WriteString("\n" + titleStyle.Render("commands") + "\n")
		b.WriteString(helpRow("space", "toggle", "run the item's on/off command; the box flips only on success"))
		if m.items[m.cursor].Check != "" {
			b.WriteString(helpRow("r", "refresh", "re-check every switch's real state"))
		}
		b.WriteString(helpRow("↑/↓, j/k", "move", ""))
		b.WriteString(helpRow("q, esc", "quit", ""))
	}

	var help string
	if m.quitArmed {
		hint := fmt.Sprintf("press %s again to quit", m.quitKey)
		if m.busy > 0 {
			hint = fmt.Sprintf("%d command(s) still running — press %s again to quit and kill them", m.busy, m.quitKey)
		}
		help = warnStyle.Render(hint)
	} else {
		hints := []string{hint("↑/↓", "move"), hint("space", "toggle")}
		if m.items[m.cursor].Check != "" {
			hints = append(hints, hint("r", "refresh"))
		}
		hints = append(hints, hint("q", "quit"), hint("?", "help"))
		help = strings.Join(hints, dimStyle.Render(" · "))
	}
	b.WriteString("\n" + help + "\n")
	return b.String()
}

func renderRow(width int, base, desc string, right string, rightStyle lipgloss.Style) string {
	const gap = 2
	baseW := lipgloss.Width(base)

	if maxRight := width - baseW - gap; lipgloss.Width(right) > maxRight {
		if maxRight <= 0 {
			right = ""
		} else {
			right = ansi.Truncate(right, maxRight, "…")
		}
	}
	rightW := lipgloss.Width(right)

	if desc != "" {
		maxDesc := width - baseW - gap - rightW
		if rightW > 0 {
			maxDesc -= gap
		}
		if maxDesc <= 1 {
			desc = ""
		} else if lipgloss.Width(desc) > maxDesc {
			desc = ansi.Truncate(desc, maxDesc, "…")
		}
	}

	line := base
	if desc != "" {
		line += "  " + dimStyle.Render(desc)
	}
	if right != "" {
		pad := max(width-lipgloss.Width(line)-rightW, 1)
		line += strings.Repeat(" ", pad) + rightStyle.Render(right)
	}
	return line
}

func hint(key, label string) string {
	return keyStyle.Render(key) + " " + dimStyle.Render(label)
}

func helpRow(keys, name, desc string) string {
	row := "  " + keyStyle.Render(fmt.Sprintf("%-10s", keys)) + name
	if desc != "" {
		row += dimStyle.Render("  — " + desc)
	}
	return row + "\n"
}

func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(100 * time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

func lastLine(s string) string {
	lines := tail(s, 1)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func tail(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
