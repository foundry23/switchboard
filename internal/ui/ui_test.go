package ui

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/foundry23/switchboard/internal/spec"
)

func newTestModel(t *testing.T, items ...spec.Item) Model {
	t.Helper()
	return New(t.Context(), spec.Board{Title: "test", Items: items}, "sh")
}

func key(k tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: k} }

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestToggleRunsCommandAndFlipsOnSuccess(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", On: "true", Off: "true"})
	next, cmd := m.Update(key(tea.KeySpace))
	nm := next.(Model)
	if !nm.items[0].busy {
		t.Fatal("expected item to be busy")
	}
	if nm.items[0].Checked {
		t.Fatal("state must not flip before the command succeeds")
	}
	if cmd == nil {
		t.Fatal("expected a command to be issued")
	}

	if !strings.Contains(nm.View(), "running · 0s") {
		t.Error("busy item should show a status line while the toggle runs")
	}

	next, _ = nm.Update(resultMsg{index: 0, target: true, output: "created worktree\n"})
	nm = next.(Model)
	if nm.items[0].busy {
		t.Error("item should no longer be busy")
	}
	if !nm.items[0].Checked {
		t.Error("expected item to be checked after success")
	}
	if !strings.HasPrefix(nm.items[0].okMsg, "✓ ") || strings.Contains(nm.items[0].okMsg, "turned") {
		t.Errorf("okMsg = %q, want a plain duration note", nm.items[0].okMsg)
	}
	if !strings.Contains(nm.View(), "created worktree") {
		t.Error("the command's own output should be shown under the item")
	}
}

func TestBusyStatusShowsLiveOutput(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", On: "true"})
	next, _ := m.Update(key(tea.KeySpace))
	nm := next.(Model)
	nm.items[0].live.Write([]byte("cloning repo\nchecking out branch\n"))
	if !strings.Contains(nm.View(), "checking out branch · 0s") {
		t.Error("busy status should relay the command's latest output line")
	}
}

func TestFailureKeepsStateAndRecordsError(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", On: "false"})
	next, _ := m.Update(key(tea.KeySpace))
	nm := next.(Model)

	next, _ = nm.Update(resultMsg{index: 0, target: true, err: errors.New("exit status 1"), output: "boom\n"})
	nm = next.(Model)
	if nm.items[0].Checked {
		t.Error("state must not flip on failure")
	}
	if !strings.Contains(nm.items[0].errMsg, "failed: exit status 1") {
		t.Errorf("errMsg = %q", nm.items[0].errMsg)
	}
	if !strings.Contains(nm.View(), "boom") {
		t.Error("failed command output should be shown")
	}
}

func TestSuccessNoteExpires(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", On: "true"})
	next, _ := m.Update(key(tea.KeySpace))
	nm := next.(Model)

	next, cmd := nm.Update(resultMsg{index: 0, target: true})
	nm = next.(Model)
	if nm.items[0].okMsg == "" {
		t.Fatal("expected a success note")
	}
	if cmd == nil {
		t.Fatal("a success should schedule the note expiry")
	}

	next, _ = nm.Update(clearNoteMsg{index: 0, seq: nm.items[0].noteSeq})
	nm = next.(Model)
	if nm.items[0].okMsg != "" {
		t.Error("the note should be cleared after the timeout")
	}
	if !nm.items[0].Checked {
		t.Error("expiring the note must not touch the checked state")
	}
}

func TestStaleNoteTimerDoesNotClearNewerNote(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", On: "true", Off: "true"})
	next, _ := m.Update(key(tea.KeySpace))
	nm := next.(Model)
	next, _ = nm.Update(resultMsg{index: 0, target: true}) // first success, seq 1
	nm = next.(Model)
	staleSeq := nm.items[0].noteSeq

	next, _ = nm.Update(key(tea.KeySpace)) // toggle back off
	nm = next.(Model)
	next, _ = nm.Update(resultMsg{index: 0, target: false}) // second success, seq 2
	nm = next.(Model)

	next, _ = nm.Update(clearNoteMsg{index: 0, seq: staleSeq})
	nm = next.(Model)
	if nm.items[0].okMsg == "" {
		t.Error("a stale timer must not clear the newer note")
	}
}

func TestErrorNoteDoesNotExpire(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", On: "false"})
	next, _ := m.Update(key(tea.KeySpace))
	nm := next.(Model)
	next, cmd := nm.Update(resultMsg{index: 0, target: true, err: errors.New("exit status 1"), output: "boom\n"})
	nm = next.(Model)
	if cmd != nil {
		t.Error("failures must not schedule an expiry")
	}
	next, _ = nm.Update(clearNoteMsg{index: 0, seq: nm.items[0].noteSeq})
	nm = next.(Model)
	if nm.items[0].errMsg == "" {
		t.Error("error notes must persist")
	}
}

func TestRowsStayOnOneLineAndTruncate(t *testing.T) {
	m := newTestModel(t, spec.Item{
		Label:       "worktree/ENG-5280",
		Description: strings.Repeat("a very long description ", 5),
		On:          "true",
	})
	m.width = 60
	next, _ := m.Update(key(tea.KeySpace))
	nm := next.(Model)
	nm.items[0].live.Write([]byte("cloning the repository\n"))

	view := nm.View()
	if !strings.Contains(view, "…") {
		t.Error("long description should be truncated with an ellipsis")
	}
	itemLines := 0
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line wider than terminal (%d): %q", w, line)
		}
		if strings.Contains(line, "worktree/ENG-5280") {
			itemLines++
			plain := strings.TrimRight(stripAnsi(line), " ")
			if !strings.HasSuffix(plain, "· 0s") {
				t.Errorf("status should be right-aligned on the item line, got %q", plain)
			}
			if !strings.Contains(plain, "cloning the repository") {
				t.Errorf("live output should be on the item line, got %q", plain)
			}
		}
	}
	if itemLines != 1 {
		t.Errorf("item rendered on %d lines, want exactly 1", itemLines)
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripAnsi(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestHelpPanelListsCommands(t *testing.T) {
	board := spec.Board{Title: "test", Items: []spec.Item{{Label: "a", Check: "true"}}}
	m := New(t.Context(), board, "sh")
	if strings.Contains(m.View(), "re-check every switch") {
		t.Fatal("help panel should be hidden by default")
	}
	next, _ := m.Update(runeKey('?'))
	nm := next.(Model)
	view := nm.View()
	for _, want := range []string{"commands", "toggle", "refresh", "re-check every switch"} {
		if !strings.Contains(view, want) {
			t.Errorf("help panel missing %q", want)
		}
	}
	next, _ = nm.Update(runeKey('?'))
	nm = next.(Model)
	if strings.Contains(nm.View(), "re-check every switch") {
		t.Error("second ? should close the help panel")
	}
}

func TestKeysAreCaseInsensitive(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a"}, spec.Item{Label: "b"})
	next, _ := m.Update(runeKey('J'))
	nm := next.(Model)
	if nm.cursor != 1 {
		t.Error("uppercase J should move like j")
	}

	m = newTestModel(t, spec.Item{Label: "a"})
	next, _ = m.Update(runeKey('Q'))
	nm = next.(Model)
	if !nm.quitArmed {
		t.Fatal("uppercase Q should arm the quit confirmation")
	}
	_, cmd := nm.Update(runeKey('Q'))
	if cmd == nil {
		t.Fatal("second uppercase Q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected a quit message")
	}
}

func TestCoalescedKeypressesAreReplayedIndividually(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a"}, spec.Item{Label: "b"}, spec.Item{Label: "c"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	nm := next.(Model)
	if nm.cursor != 2 {
		t.Errorf("cursor = %d after coalesced jj, want 2", nm.cursor)
	}
}

func TestStartupChecksProbeState(t *testing.T) {
	m := newTestModel(t,
		spec.Item{Label: "a", Check: "true", Checked: false},
		spec.Item{Label: "b", Check: "true", Checked: true},
	)
	if m.busy != 2 {
		t.Fatalf("busy = %d, want both items probing at startup", m.busy)
	}
	if m.Init() == nil {
		t.Fatal("Init should fire the startup probes")
	}
	if !strings.Contains(m.View(), "checking") {
		t.Error("probing items should show a checking status")
	}

	next, _ := m.Update(resultMsg{index: 0, check: true})
	nm := next.(Model)
	next, _ = nm.Update(resultMsg{index: 1, check: true, err: errors.New("exit status 1")})
	nm = next.(Model)
	if !nm.items[0].Checked {
		t.Error("probe exit 0 should mean checked")
	}
	if nm.items[1].Checked {
		t.Error("probe non-zero exit should mean unchecked, overriding the seed")
	}
	if nm.items[1].errMsg != "" {
		t.Errorf("a probe's non-zero exit is state, not an error, got %q", nm.items[1].errMsg)
	}
	if nm.busy != 0 {
		t.Errorf("busy = %d after both probes", nm.busy)
	}
}

func TestNoChecksMeansNoStartupProbes(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", Checked: true})
	if m.busy != 0 || m.Init() != nil {
		t.Error("items without a check command should trust the spec's checked state")
	}
	if !m.items[0].Checked {
		t.Error("seed state should be kept")
	}
}

func TestRefreshReprobesIdleItems(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", Check: "true"})
	next, _ := m.Update(resultMsg{index: 0, check: true}) // startup probe done
	nm := next.(Model)
	if nm.busy != 0 {
		t.Fatalf("busy = %d, want idle before refresh", nm.busy)
	}

	next, cmd := nm.Update(runeKey('r'))
	nm = next.(Model)
	if nm.busy != 1 || cmd == nil {
		t.Error("r should re-probe items with a check command")
	}

	m2 := newTestModel(t, spec.Item{Label: "a"})
	_, cmd = m2.Update(runeKey('r'))
	if cmd != nil {
		t.Error("r should be a no-op when nothing has a check command")
	}
}

func TestUnboundKeyIgnored(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a"})
	next, cmd := m.Update(runeKey('z'))
	nm := next.(Model)
	if cmd != nil || nm.items[0].busy {
		t.Error("unbound key should be a no-op")
	}
}

func TestToggleWhileBusyIgnored(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", On: "sleep 1"})
	next, _ := m.Update(key(tea.KeySpace))
	nm := next.(Model)
	next, cmd := nm.Update(key(tea.KeySpace))
	nm = next.(Model)
	if cmd != nil {
		t.Error("toggling a busy item should be a no-op")
	}
	if nm.busy != 1 {
		t.Errorf("busy = %d, want 1", nm.busy)
	}
}

func TestNavigationStaysInBounds(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a"}, spec.Item{Label: "b"})
	next, _ := m.Update(key(tea.KeyUp))
	nm := next.(Model)
	if nm.cursor != 0 {
		t.Errorf("cursor = %d after up at top, want 0", nm.cursor)
	}
	next, _ = nm.Update(runeKey('j'))
	nm = next.(Model)
	next, _ = nm.Update(runeKey('j'))
	nm = next.(Model)
	if nm.cursor != 1 {
		t.Errorf("cursor = %d after down at bottom, want 1", nm.cursor)
	}
}

func TestQuitRequiresConfirmation(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a"})
	next, cmd := m.Update(runeKey('q'))
	nm := next.(Model)
	if !nm.quitArmed {
		t.Fatal("first q should arm the quit confirmation")
	}
	if cmd == nil {
		t.Fatal("first q should schedule the disarm timeout")
	}
	if !strings.Contains(nm.View(), "press q again to quit") {
		t.Error("the confirmation hint should be shown")
	}

	_, cmd = nm.Update(runeKey('q'))
	if cmd == nil {
		t.Fatal("second q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected a quit message")
	}
}

func TestQuitConfirmationMentionsRunningCommands(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a", On: "sleep 1"})
	next, _ := m.Update(key(tea.KeySpace))
	nm := next.(Model)
	next, _ = nm.Update(runeKey('q'))
	nm = next.(Model)
	if !strings.Contains(nm.View(), "still running") {
		t.Error("confirmation should warn that commands will be killed")
	}

	_, cmd := nm.Update(key(tea.KeyEsc))
	if cmd == nil {
		t.Fatal("any quit key should confirm an armed quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected a quit message")
	}
}

func TestQuitDisarmsOnTimeoutAndOtherKeys(t *testing.T) {
	m := newTestModel(t, spec.Item{Label: "a"})
	next, _ := m.Update(runeKey('q'))
	nm := next.(Model)

	next, _ = nm.Update(quitTimeoutMsg{seq: nm.quitSeq})
	nm = next.(Model)
	if nm.quitArmed {
		t.Fatal("timeout should disarm the confirmation")
	}

	next, _ = nm.Update(runeKey('q'))
	nm = next.(Model)
	next, _ = nm.Update(runeKey('z'))
	nm = next.(Model)
	if nm.quitArmed {
		t.Fatal("any other key should disarm the confirmation")
	}

	// A stale timeout from a disarmed confirmation must not cancel a new one.
	next, _ = nm.Update(runeKey('q'))
	nm = next.(Model)
	next, _ = nm.Update(quitTimeoutMsg{seq: nm.quitSeq - 1})
	nm = next.(Model)
	if !nm.quitArmed {
		t.Fatal("a stale timeout must not disarm a newer confirmation")
	}
}
