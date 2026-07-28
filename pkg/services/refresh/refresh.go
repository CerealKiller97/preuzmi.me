// Package refresh runs the receipt download across every configured provider.
//
// It backs both the `checks` CLI command and the refresh button in the UI, so
// the two can never drift apart.
package refresh

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
)

const fileName = "refresh.json"

// Result is the outcome of one provider's download.
//
// Period and Price are filled in after the run from the receipts database, when
// available, so notifications can report which month's bill was fetched and how
// much it was.
type Result struct {
	Provider   string  `json:"provider"`
	Error      string  `json:"error,omitempty"`
	Period     string  `json:"period,omitempty"`
	DurationMS int64   `json:"duration_ms"`
	Price      float64 `json:"price,omitempty"`
	OK         bool    `json:"ok"`
	// New reports whether this run downloaded the receipt for the first time, as
	// opposed to re-downloading one already on record. It is filled in after the
	// run (from the receipts database) and drives the download notification, so a
	// daily re-run does not re-announce an already-downloaded bill.
	New bool `json:"new,omitempty"`
}

// State is what the UI needs to render the refresh control.
type State struct {
	Results    []Result `json:"results"`
	StartedAt  int64    `json:"started_at"`
	FinishedAt int64    `json:"finished_at"`
	CheckUntil int      `json:"check_until"`
	Running    bool     `json:"running"`
	Allowed    bool     `json:"allowed"`
}

// Service coordinates refreshes and remembers when the last one finished.
type Service struct {
	path  string
	state State
	mu    sync.Mutex
}

// New loads the last recorded refresh from dir.
//
// A run that was in flight when the process died is reported as finished:
// nothing is running after a restart.
func New(dir string) (*Service, error) {
	s := &Service{path: filepath.Join(dir, fileName)}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}

		return nil, err
	}

	if len(strings.TrimSpace(string(b))) == 0 {
		return s, nil
	}

	if err := json.Unmarshal(b, &s.state); err != nil {
		return nil, err
	}

	s.state.Running = false

	return s, nil
}

// State returns a copy of the current state.
func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := s.state
	out.Results = append([]Result(nil), s.state.Results...)

	return out
}

// ErrAlreadyRunning is returned when a refresh is already in flight.
var ErrAlreadyRunning = errors.New("a refresh is already running")

// ErrOutsideWindow is returned when today is past the configured check_until
// day of the month.
var ErrOutsideWindow = errors.New("refresh is only allowed until check_until day of the month")

// WithWindow stamps the check_until window onto a copy of the state so the UI
// can disable the button without a second round-trip.
func (s State) WithWindow(checkUntil int, allowed bool) State {
	s.CheckUntil = checkUntil
	s.Allowed = allowed

	return s
}

// Start kicks off a refresh in the background and returns immediately.
//
// Only one refresh may run at a time: these calls log into real provider
// accounts, so overlapping runs risk hammering them or tripping rate limits.
//
// after, when non-nil, is called once the run finishes (with the results)
// still on the worker goroutine — used for notifications.
func (s *Service) Start(providers map[string]provider.Interface, after func([]Result)) (State, error) {
	s.mu.Lock()

	if s.state.Running {
		out := s.state
		s.mu.Unlock()

		return out, ErrAlreadyRunning
	}

	s.state = State{
		Running:   true,
		StartedAt: time.Now().Unix(),
	}
	current := s.state
	s.mu.Unlock()

	go func() {
		results := Run(providers)
		s.finish(results)

		if after != nil {
			after(results)
		}
	}()

	return current, nil
}

// RunOnce downloads from every provider synchronously and records the run, so
// the persisted "last refresh" time advances after the `checks` command too —
// not only after the UI's refresh button. Without this, a scheduled `checks`
// run would download bills yet leave the dashboard showing a stale time, making
// it look as though nothing happened.
//
// Unlike Start it blocks until the run finishes and returns the results, which
// suits the one-shot `checks` command that exits when it is done.
func (s *Service) RunOnce(providers map[string]provider.Interface) ([]Result, error) {
	s.mu.Lock()
	if s.state.Running {
		s.mu.Unlock()
		return nil, ErrAlreadyRunning
	}
	s.state = State{
		Running:   true,
		StartedAt: time.Now().Unix(),
	}
	s.mu.Unlock()

	results := Run(providers)
	s.finish(results)

	return results, nil
}

// finish records a completed run: it stamps the finish time, stores the results
// and persists them to disk. Shared by Start and RunOnce so both entry points
// leave identical state behind.
func (s *Service) finish(results []Result) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Running = false
	s.state.FinishedAt = time.Now().Unix()
	s.state.Results = results
	_ = s.persist()
}

// SyncFromDisk reloads the persisted state when a newer run finished out of
// process — the `checks` cron writes refresh.json from a separate process, and
// a long-running server would otherwise keep serving the state it loaded at
// startup. A run in flight in this process is authoritative and left untouched.
func (s *Service) SyncFromDisk() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Running {
		return
	}

	b, err := os.ReadFile(s.path)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return
	}

	var disk State
	if err := json.Unmarshal(b, &disk); err != nil {
		return
	}

	// Adopt the on-disk run only when it is newer, so we never regress to an
	// older state or clobber a fresher in-memory one.
	if disk.FinishedAt > s.state.FinishedAt {
		disk.Running = false
		s.state = disk
	}
}

// Run downloads from every provider concurrently and reports each outcome.
//
// One provider failing never stops the others: a broken eSanduce login should
// not cost you the MTS receipt.
func Run(providers map[string]provider.Interface) []Result {
	results := make([]Result, 0, len(providers))

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	for name, p := range providers {
		wg.Add(1)

		go func(name string, p provider.Interface) {
			defer wg.Done()

			started := time.Now()
			err := p.DownloadReceipt()

			result := Result{
				Provider:   name,
				OK:         err == nil,
				DurationMS: time.Since(started).Milliseconds(),
			}
			if err != nil {
				result.Error = err.Error()
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(name, p)
	}

	wg.Wait()

	// Stable order, since goroutines finish in whatever order they please.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Provider < results[j].Provider
	})

	return results
}

// persist writes the last run to disk. The caller must hold the lock.
func (s *Service) persist() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, fileName+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), s.path)
}
