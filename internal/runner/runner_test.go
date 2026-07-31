package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesOutput(t *testing.T) {
	out, err := Run(t.Context(), "sh", "echo out; echo err >&2", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "out") || !strings.Contains(out, "err") {
		t.Errorf("output = %q, want stdout and stderr combined", out)
	}
}

func TestRunReportsFailure(t *testing.T) {
	out, err := Run(t.Context(), "sh", "echo boom >&2; exit 3", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "3") {
		t.Errorf("err = %v, want exit status 3", err)
	}
	if !strings.Contains(out, "boom") {
		t.Errorf("output = %q, want failure output preserved", out)
	}
}

func TestRunKilledOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := Run(ctx, "sh", "sleep 30", nil)
	if err == nil {
		t.Fatal("expected an error after cancellation")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v, command was not killed promptly", elapsed)
	}
}

func TestRunStreamsOutputWhileRunning(t *testing.T) {
	live := &Live{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Prints a line, then blocks long enough for the assertion below.
		if _, err := Run(t.Context(), "sh", "echo phase one; sleep 2", live); err != nil {
			t.Errorf("Run: %v", err)
		}
	}()

	deadline := time.After(1500 * time.Millisecond)
	for live.LastLine() != "phase one" {
		select {
		case <-deadline:
			t.Fatalf("live output = %q, want %q before the command finishes", live.LastLine(), "phase one")
		case <-time.After(10 * time.Millisecond):
		}
	}
	<-done
}

func TestLiveLastLine(t *testing.T) {
	live := &Live{}
	if live.LastLine() != "" {
		t.Errorf("empty buffer LastLine = %q", live.LastLine())
	}
	live.Write([]byte("one\ntwo\n"))
	if got := live.LastLine(); got != "two" {
		t.Errorf("LastLine = %q, want two", got)
	}
	live.Write([]byte("thr"))
	if got := live.LastLine(); got != "thr" {
		t.Errorf("LastLine = %q, want partial line thr", got)
	}
	live.Write([]byte("ee\rprogress 50%"))
	if got := live.LastLine(); got != "progress 50%" {
		t.Errorf("LastLine = %q, want text after carriage return", got)
	}
}
