// Package jobs runs one long operation at a time and broadcasts its output to
// every connected browser.
//
// This is the server-side half of the old manager's SetBusy(): while a job is
// running, every other operation is refused. The old manager enforced that by
// greying out buttons, which is not enough for a web UI where a second tab (or
// a reloaded page) can post the same request again. The mutex here is the real
// guard; the UI disabling is only a courtesy.
package jobs

import (
	"errors"
	"strconv"
	"sync"
	"time"
)

// ErrBusy is returned when a job is already running.
var ErrBusy = errors.New("別の処理を実行中です。完了してから操作してください")

// Event kinds beyond the runner's cmd/out/err.
const (
	KindInfo  = "info"  // manager's own commentary
	KindStart = "start" // a job began
	KindEnd   = "end"   // a job finished (Text carries the outcome)
)

// Event is one line in the log pane.
type Event struct {
	Seq  int64  `json:"seq"`
	Time string `json:"time"` // HH:MM:SS, matching the old log pane
	Kind string `json:"kind"`
	Text string `json:"text"`
	Job  string `json:"job,omitempty"`
}

// Status describes the current or most recent job.
type Status struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Started  string `json:"started,omitempty"`
	Finished string `json:"finished,omitempty"`
	Failed   bool   `json:"failed"`
	Error    string `json:"error,omitempty"`
}

// ringSize caps the replay buffer handed to a browser that connects late. The
// manager is normally started before the browser is opened, so without a replay
// the first page load would show an empty log.
const ringSize = 2000

// subBuffer is how far a single SSE client may fall behind before events are
// dropped for it. Dropping keeps one stalled browser tab from blocking a job.
const subBuffer = 512

// Manager owns the single job slot and the subscriber set.
type Manager struct {
	mu      sync.Mutex
	running bool
	status  Status
	seq     int64
	nextID  int64
	ring    []Event
	subs    map[chan Event]struct{}
}

// New creates an idle Manager.
func New() *Manager {
	return &Manager{subs: make(map[chan Event]struct{})}
}

// Log is handed to a job so it can write to the log pane.
type Log struct {
	m  *Manager
	id string
}

// Emit publishes one line of the given kind.
func (l *Log) Emit(kind, text string) { l.m.publish(kind, text, l.id) }

// Info publishes a line from the manager itself.
func (l *Log) Info(text string) { l.Emit(KindInfo, text) }

// Start begins a job unless one is already running. fn runs on its own
// goroutine; Start returns as soon as the job is accepted.
func (m *Manager) Start(name string, fn func(*Log) error) (Status, error) {
	m.mu.Lock()
	if m.running {
		st := m.status
		m.mu.Unlock()
		return st, ErrBusy
	}
	m.nextID++
	id := strconv.FormatInt(m.nextID, 10)
	m.running = true
	m.status = Status{ID: id, Name: name, Running: true, Started: time.Now().Format(time.RFC3339)}
	st := m.status
	m.mu.Unlock()

	m.publish(KindStart, name, id)

	go func() {
		log := &Log{m: m, id: id}
		err := fn(log)

		m.mu.Lock()
		m.status.Running = false
		m.status.Finished = time.Now().Format(time.RFC3339)
		m.status.Failed = err != nil
		if err != nil {
			m.status.Error = err.Error()
		} else {
			m.status.Error = ""
		}
		m.running = false
		m.mu.Unlock()

		if err != nil {
			m.publish(KindEnd, name+" は失敗しました: "+err.Error(), id)
			return
		}
		m.publish(KindEnd, name+" が完了しました", id)
	}()

	return st, nil
}

// Busy reports whether a job is running. Handlers call this to refuse work
// before doing anything with side effects.
func (m *Manager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Status returns the current or most recent job status.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Announce publishes a line that does not belong to any job, such as the
// startup banner.
func (m *Manager) Announce(text string) { m.publish(KindInfo, text, "") }

func (m *Manager) publish(kind, text, job string) {
	m.mu.Lock()
	m.seq++
	ev := Event{
		Seq:  m.seq,
		Time: time.Now().Format("15:04:05"),
		Kind: kind,
		Text: text,
		Job:  job,
	}
	m.ring = append(m.ring, ev)
	if len(m.ring) > ringSize {
		m.ring = m.ring[len(m.ring)-ringSize:]
	}
	// Delivery happens under the lock. Every send is non-blocking, so this
	// cannot stall, and it keeps a subscriber from being closed by Subscribe's
	// cancel func while we are mid-send.
	for ch := range m.subs {
		select {
		case ch <- ev:
		default: // subscriber is too far behind; drop rather than stall the job
		}
	}
	m.mu.Unlock()
}

// Subscribe returns a channel of future events plus the events already in the
// replay buffer. The returned function must be called to release the channel.
func (m *Manager) Subscribe() (<-chan Event, []Event, func()) {
	ch := make(chan Event, subBuffer)
	m.mu.Lock()
	m.subs[ch] = struct{}{}
	backlog := make([]Event, len(m.ring))
	copy(backlog, m.ring)
	m.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.subs, ch)
			m.mu.Unlock()
			close(ch)
		})
	}
	return ch, backlog, cancel
}
