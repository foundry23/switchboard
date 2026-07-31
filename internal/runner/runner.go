package runner

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// waitDelay bounds how long Run waits for the command's pipes to close after
// cancellation before the process is force-killed.
const waitDelay = 5 * time.Second

type Live struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *Live) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// String returns everything written so far.
func (l *Live) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// LastLine returns the most recent (possibly still unfinished) line.
func (l *Live) LastLine() string {
	s := strings.TrimRight(l.String(), "\r\n")
	if i := strings.LastIndexAny(s, "\r\n"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

// Run executes command as `shell -c command` and returns its combined output.
// Output is streamed into live as it is produced (pass nil when live progress
// is not needed). Item args are substituted into the command text before it
// reaches Run (see spec.ExpandArgs). The command gets its own process group
// and cancelling ctx terminates the whole group, so children started by the
// shell don't outlive the TUI.
func Run(ctx context.Context, shell, command string, live *Live) (string, error) {
	if live == nil {
		live = &Live{}
	}
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = waitDelay
	cmd.Stdout = live
	cmd.Stderr = live
	err := cmd.Run()
	return live.String(), err
}
