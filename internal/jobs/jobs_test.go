package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func TestOnlyOneJobRunsAtATime(t *testing.T) {
	m := New()
	release := make(chan struct{})
	if _, err := m.Start("first", func(ctx context.Context, l *Log) error {
		<-release
		return nil
	}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	waitFor(t, m.Busy, "the first job to be marked running")

	if _, err := m.Start("second", func(ctx context.Context, l *Log) error { return nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Start error = %v, want ErrBusy", err)
	}

	close(release)
	waitFor(t, func() bool { return !m.Busy() }, "the first job to finish")

	if _, err := m.Start("third", func(ctx context.Context, l *Log) error { return nil }); err != nil {
		t.Fatalf("Start after completion: %v", err)
	}
}

func TestSubscriberReceivesLinesWhileJobRuns(t *testing.T) {
	m := New()
	ch, backlog, cancel := m.Subscribe()
	defer cancel()
	if len(backlog) != 0 {
		t.Fatalf("backlog = %d, want 0 on a fresh manager", len(backlog))
	}

	step := make(chan struct{})
	if _, err := m.Start("streaming", func(ctx context.Context, l *Log) error {
		l.Info("one")
		<-step
		l.Info("two")
		return nil
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The lines must arrive as they are produced, not after the job returns.
	if ev := recv(t, ch); ev.Kind != KindStart {
		t.Fatalf("first event kind = %q, want %q", ev.Kind, KindStart)
	}
	if ev := recv(t, ch); ev.Text != "one" {
		t.Fatalf("second event = %q, want %q", ev.Text, "one")
	}
	close(step)
	if ev := recv(t, ch); ev.Text != "two" {
		t.Fatalf("third event = %q, want %q", ev.Text, "two")
	}
	if ev := recv(t, ch); ev.Kind != KindEnd {
		t.Fatalf("final event kind = %q, want %q", ev.Kind, KindEnd)
	}
}

// A job that hangs must be stoppable, otherwise the single job slot is lost
// for the life of the process.
func TestCancelStopsTheRunningJob(t *testing.T) {
	m := New()
	if got := m.Cancel(); got {
		t.Error("Cancel with nothing running = true")
	}

	started := make(chan struct{})
	if _, err := m.Start("hanging", func(ctx context.Context, l *Log) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	if got := m.Cancel(); !got {
		t.Fatal("Cancel while running = false")
	}
	waitFor(t, func() bool { return !m.Busy() }, "the cancelled job to finish")

	if st := m.Status(); !st.Failed {
		t.Errorf("status = %+v, want Failed after cancellation", st)
	}
	// The slot has to be free again.
	if _, err := m.Start("next", func(ctx context.Context, l *Log) error { return nil }); err != nil {
		t.Errorf("Start after cancellation: %v", err)
	}
}

func TestFailedJobIsReportedInStatusAndLog(t *testing.T) {
	m := New()
	ch, _, cancel := m.Subscribe()
	defer cancel()

	if _, err := m.Start("failing", func(ctx context.Context, l *Log) error { return errors.New("boom") }); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, func() bool { return !m.Busy() }, "the job to finish")

	st := m.Status()
	if !st.Failed || st.Error != "boom" {
		t.Errorf("status = %+v, want Failed with error boom", st)
	}

	var sawEnd bool
	for i := 0; i < 2; i++ {
		if ev := recv(t, ch); ev.Kind == KindEnd {
			sawEnd = true
			if ev.Text == "" {
				t.Error("end event should carry the failure text")
			}
		}
	}
	if !sawEnd {
		t.Error("no end event was published")
	}
}

// A browser opened after the manager started must still see what happened.
func TestLateSubscriberGetsBacklog(t *testing.T) {
	m := New()
	m.Announce("startup banner")
	waitFor(t, func() bool { return true }, "nothing")

	_, backlog, cancel := m.Subscribe()
	defer cancel()
	if len(backlog) != 1 || backlog[0].Text != "startup banner" {
		t.Fatalf("backlog = %+v, want the announce line", backlog)
	}
}

// A stalled browser tab must not block a running job.
func TestSlowSubscriberDoesNotBlockPublishing(t *testing.T) {
	m := New()
	_, _, cancel := m.Subscribe() // never drained
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < subBuffer*3; i++ {
			m.Announce("flood")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("publishing blocked on an undrained subscriber")
	}
}

func recv(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an event")
		return Event{}
	}
}
