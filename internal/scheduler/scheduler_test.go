package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amishk599/firstin/internal/model"
	"github.com/amishk599/firstin/internal/poller"
)

// --- Mock implementations ---

type CountingFetcher struct {
	calls atomic.Int32
}

func (f *CountingFetcher) FetchJobs(_ context.Context) ([]model.Job, error) {
	f.calls.Add(1)
	return nil, nil
}

type ErrorFetcher struct {
	calls atomic.Int32
}

func (f *ErrorFetcher) FetchJobs(_ context.Context) ([]model.Job, error) {
	f.calls.Add(1)
	return nil, errors.New("fetch failed")
}

// OrderRecordingFetcher appends its id to recorder.order on each FetchJobs call.
// Used to assert poll order within an ATS group.
type OrderRecordingFetcher struct {
	id       string
	recorder *orderRecorder
}

type orderRecorder struct {
	mu    sync.Mutex
	order []string
}

func (f *OrderRecordingFetcher) FetchJobs(_ context.Context) ([]model.Job, error) {
	f.recorder.mu.Lock()
	f.recorder.order = append(f.recorder.order, f.id)
	f.recorder.mu.Unlock()
	return nil, nil
}

type NoOpStore struct{}

func (s *NoOpStore) HasSeen(_ string) (bool, error) { return false, nil }
func (s *NoOpStore) MarkSeen(_ string) error         { return nil }
func (s *NoOpStore) Cleanup(_ time.Duration) error   { return nil }
func (s *NoOpStore) IsEmpty() (bool, error)          { return false, nil }

type NoOpNotifier struct{}

func (n *NoOpNotifier) Notify(_ []model.Job) error { return nil }

type AcceptAllFilter struct{}

func (f *AcceptAllFilter) Match(_ model.Job) bool { return true }

type NopAnalyzer struct{}

func (n *NopAnalyzer) Analyze(_ context.Context, job model.Job) (model.Job, error) {
	return job, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makePoller(name, ats string, fetcher model.JobFetcher) *poller.CompanyPoller {
	return poller.NewCompanyPoller(
		name,
		ats,
		fetcher,
		&AcceptAllFilter{},
		&NoOpStore{},
		&NoOpNotifier{},
		&NopAnalyzer{},
		time.Hour,
		discardLogger(),
	)
}

// --- Tests ---

func TestGroupByATS(t *testing.T) {
	pollers := []*poller.CompanyPoller{
		makePoller("co1", "greenhouse", &CountingFetcher{}),
		makePoller("co2", "ashby", &CountingFetcher{}),
		makePoller("co3", "greenhouse", &CountingFetcher{}),
	}
	s := NewScheduler(pollers, time.Hour, 0, nil, discardLogger())
	groups := s.groupByATS()

	if len(groups) != 2 {
		t.Fatalf("groupByATS: got %d groups, want 2", len(groups))
	}
	if len(groups["greenhouse"]) != 2 {
		t.Errorf("greenhouse group: got %d pollers, want 2", len(groups["greenhouse"]))
	}
	if len(groups["ashby"]) != 1 {
		t.Errorf("ashby group: got %d pollers, want 1", len(groups["ashby"]))
	}
}

func TestRun_CancelReturnsPromptly(t *testing.T) {
	p := makePoller("testco", "greenhouse", &CountingFetcher{})
	s := NewScheduler([]*poller.CompanyPoller{p}, 1*time.Hour, time.Minute, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on cancel, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not return within 2s after cancel")
	}
}

func TestRun_PollersCalledPerATSLoop(t *testing.T) {
	fetcher := &CountingFetcher{}
	pollers := []*poller.CompanyPoller{
		makePoller("co1", "greenhouse", fetcher),
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(pollers, 100*time.Millisecond, 0, nil, discardLogger())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Allow time for at least two full passes (poll → sleep interval → poll).
	time.Sleep(250 * time.Millisecond)
	cancel()
	<-done

	if got := fetcher.calls.Load(); got < 2 {
		t.Errorf("fetcher calls = %d, want >= 2", got)
	}
}

func TestRun_OnePollerErrorOthersInDifferentATSStillRun(t *testing.T) {
	errFetcher := &ErrorFetcher{}
	okFetcher := &CountingFetcher{}

	pollers := []*poller.CompanyPoller{
		makePoller("failing", "greenhouse", errFetcher),
		makePoller("healthy", "ashby", okFetcher),
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(pollers, 1*time.Hour, 0, nil, discardLogger())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if got := errFetcher.calls.Load(); got < 1 {
		t.Errorf("error fetcher calls = %d, want >= 1", got)
	}
	if got := okFetcher.calls.Load(); got < 1 {
		t.Errorf("ok fetcher calls = %d, want >= 1 (different ATS should run independently)", got)
	}
}

func TestRun_MinDelayBetweenSameATSCompanies(t *testing.T) {
	fetcher := &CountingFetcher{}
	pollers := []*poller.CompanyPoller{
		makePoller("co1", "greenhouse", fetcher),
		makePoller("co2", "greenhouse", fetcher),
	}

	// Min delay 50ms between companies in same ATS; interval 1h so we only do one pass.
	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(pollers, 1*time.Hour, 50*time.Millisecond, nil, discardLogger())

	start := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Wait for both companies to be polled (co1, then 50ms delay, then co2).
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	elapsed := time.Since(start)
	if got := fetcher.calls.Load(); got != 2 {
		t.Errorf("fetcher calls = %d, want 2", got)
	}
	// We expect at least ~50ms between the two polls (min_delay).
	if elapsed < 50*time.Millisecond {
		t.Errorf("elapsed %v: expected >= 50ms (min_delay between same-ATS companies)", elapsed)
	}
}

func TestRun_OnePollerErrorSameATSGroupContinues(t *testing.T) {
	errFetcher := &ErrorFetcher{}
	okFetcher := &CountingFetcher{}

	// Same ATS group: first company fails, second should still be polled.
	pollers := []*poller.CompanyPoller{
		makePoller("failing", "greenhouse", errFetcher),
		makePoller("healthy", "greenhouse", okFetcher),
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(pollers, 1*time.Hour, 0, nil, discardLogger())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	if got := errFetcher.calls.Load(); got < 1 {
		t.Errorf("error fetcher calls = %d, want >= 1", got)
	}
	if got := okFetcher.calls.Load(); got < 1 {
		t.Errorf("healthy fetcher calls = %d, want >= 1 (scheduler should continue to next company in same ATS)", got)
	}
}

func TestRun_AllATSGroupsRunIndependently(t *testing.T) {
	ghFetcher := &CountingFetcher{}
	ashbyFetcher := &CountingFetcher{}
	workdayFetcher := &CountingFetcher{}

	pollers := []*poller.CompanyPoller{
		makePoller("co1", "greenhouse", ghFetcher),
		makePoller("co2", "ashby", ashbyFetcher),
		makePoller("co3", "workday", workdayFetcher),
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(pollers, 1*time.Hour, 0, nil, discardLogger())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	for name, f := range map[string]*CountingFetcher{"greenhouse": ghFetcher, "ashby": ashbyFetcher, "workday": workdayFetcher} {
		if got := f.calls.Load(); got < 1 {
			t.Errorf("%s fetcher calls = %d, want >= 1 (each ATS group runs in its own goroutine)", name, got)
		}
	}
}

func TestRun_OrderWithinGroupPreserved(t *testing.T) {
	rec := &orderRecorder{}
	pollers := []*poller.CompanyPoller{
		makePoller("co1", "greenhouse", &OrderRecordingFetcher{id: "co1", recorder: rec}),
		makePoller("co2", "greenhouse", &OrderRecordingFetcher{id: "co2", recorder: rec}),
		makePoller("co3", "greenhouse", &OrderRecordingFetcher{id: "co3", recorder: rec}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(pollers, 1*time.Hour, 0, nil, discardLogger())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// One full pass: co1, co2, co3 (no min_delay).
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	rec.mu.Lock()
	order := append([]string(nil), rec.order...)
	rec.mu.Unlock()

	want := []string{"co1", "co2", "co3"}
	if len(order) != len(want) {
		t.Fatalf("poll order length = %d, want %d (order: %v)", len(order), len(want), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("poll order = %v, want %v", order, want)
			break
		}
	}
}
