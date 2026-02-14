# FirstIn — CLAUDE.md

## What This Project Is

**FirstIn** is a job polling system that detects newly posted software engineering roles (posted within the last 1 hour) directly from company career websites. The name says it all — the goal is to be the first in the door by polling career pages on a schedule, filtering for relevant roles, and sending notifications for new matches before anyone else sees them.

## Core Problem

Every company exposes jobs differently. The architecture creates a uniform internal pipeline while allowing per-company customization at the edges via the adapter pattern.

## Non-Goals

- No scraping of job aggregators (LinkedIn, Indeed, etc.)
- Only direct polling of company career pages
- No manual browsing or cookie dependency

---

## Tech Stack

- Language: Go
- Storage: SQLite (seen-jobs tracking)
- Config: YAML (company registry + filter settings)
- Deployment: Single binary (`firstin`), runs anywhere (laptop, VPS, Raspberry Pi, Docker)

---

## Architecture — 5 Layers

```
┌─────────────────────────────────────────────────┐
│              NOTIFICATION LAYER                  │
│         (Email, Slack, Discord, etc.)            │
├─────────────────────────────────────────────────┤
│              DEDUP & FILTER LAYER                │
│    (New job detection, keyword/location filter)  │
├─────────────────────────────────────────────────┤
│             NORMALIZATION LAYER                  │
│     (Raw response → unified Job struct)          │
├─────────────────────────────────────────────────┤
│              FETCHER LAYER                       │
│  (HTTP calls, GET/POST/headers/pagination)       │
├─────────────────────────────────────────────────┤
│             SCHEDULER / ORCHESTRATOR             │
│    (Per-company polling intervals, retries)      │
└─────────────────────────────────────────────────┘
```

Data flows bottom to top: Scheduler triggers → Fetcher gets raw data → Normalizer produces `[]Job` → Filter/Dedup removes noise → Notifier alerts on new matches.

---

## V1 Scope (Build Now)

V1 is intentionally minimal: 2-3 companies, all on Greenhouse, end-to-end pipeline working and stable.

### V1 Includes

- Greenhouse adapter only (reusable across all Greenhouse companies)
- Simple sequential polling loop (no concurrency)
- Single global polling interval (e.g., 5 minutes)
- SQLite for seen-job tracking
- Title + location keyword filtering
- Log-to-stdout notifications (or single channel like Slack)
- Graceful shutdown on SIGINT/SIGTERM
- Basic error logging (log and continue on failure)

### V1 Does NOT Include

- Priority queue scheduling
- Worker pool / semaphore / concurrency control
- Circuit breaking
- Exponential backoff with jitter
- Adaptive polling intervals
- Health tracking / observability dashboards
- Dynamic config reload
- Multiple ATS adapters (Lever, Ashby, custom)

---

## Unified Job Struct

Every adapter normalizes to this single struct:

```go
type Job struct {
    ID        string     // unique per platform (Greenhouse: numeric ID, Lever: UUID)
    Company   string
    Title     string
    Location  string
    URL       string     // direct apply link
    PostedAt  *time.Time // nullable — not all APIs provide this
    FirstSeen time.Time  // our clock, set on first encounter
    Source    string     // ATS name, e.g. "greenhouse"
}
```

---

## SOLID Design — Interfaces

The system is built on small, focused interfaces. The scheduler and pipeline depend only on these contracts, never on concrete types. This is what makes every future upgrade a clean addition rather than a modification.

### JobFetcher

```go
type JobFetcher interface {
    FetchJobs(ctx context.Context) ([]Job, error)
}
```

- V1: `GreenhouseAdapter`
- Future: `LeverAdapter`, `AshbyAdapter`, `CustomScraperAdapter`
- Tests: `MockAdapter`

### JobStore

```go
type JobStore interface {
    HasSeen(jobID string) (bool, error)
    MarkSeen(jobID string) error
    Cleanup(olderThan time.Duration) error
}
```

- V1: `SQLiteStore`
- Future: `PostgresStore` if needed
- Tests: `InMemoryStore`

### Notifier

```go
type Notifier interface {
    Notify(jobs []Job) error
}
```

- V1: `LogNotifier` (prints to stdout)
- Future: `SlackNotifier`, `MultiNotifier` (fan-out to many channels)
- Tests: `RecordingNotifier`

### JobFilter

```go
type JobFilter interface {
    Match(job Job) bool
}
```

- V1: `TitleAndLocationFilter`
- Future: `CompositeFilter` (chain of filters), `RegexFilter`, `SeniorityFilter`

---

## Key Structs

### CompanyPoller

Owns the full pipeline for ONE company. This is the unit of work that a future worker pool will call.

```go
type CompanyPoller struct {
    Name     string
    Fetcher  JobFetcher  // interface
    Filter   JobFilter   // interface
    Store    JobStore    // interface
    Notifier Notifier    // interface
}
```

Single method: `Poll(ctx context.Context) error`

Poll does:
1. `fetcher.FetchJobs(ctx)` → get all jobs
2. Filter each job through `filter.Match(job)`
3. Dedup each filtered job via `store.HasSeen(job.ID)`
4. If new jobs exist: `notifier.Notify(newJobs)`
5. Mark all new jobs as seen: `store.MarkSeen(job.ID)`

### Scheduler

Owns the main loop, timing, and shutdown. Does NOT know about HTTP, JSON, databases, or notifications.

```go
type Scheduler struct {
    Pollers  []CompanyPoller
    Interval time.Duration
    Logger   *slog.Logger
}
```

Single method: `Run(ctx context.Context) error`

---

## V1 Main Loop

```
loop:
  for each company in config:
    poller.Poll(ctx)
      on error → log it, move to next company
    small sleep between companies (1-2 sec)
  sleep(polling_interval)
  check for shutdown signal → if yes, exit cleanly
```

Sequential. Simple. Sufficient for 2-3 companies.

---

## V1 Wiring (main.go)

`main()` is the ONLY place that knows about concrete types. Everything below works with interfaces (Dependency Inversion).

```
main()
  ├── load config from YAML
  ├── store    = NewSQLiteStore("jobs.db")
  ├── notifier = NewLogNotifier()
  ├── filter   = NewTitleLocationFilter(config)
  ├── for each company in config:
  │     ├── adapter = NewGreenhouseAdapter(boardToken, httpClient)
  │     └── poller  = NewCompanyPoller(name, adapter, filter, store, notifier)
  ├── scheduler = NewScheduler(pollers, interval, logger)
  └── scheduler.Run(ctx)  ← blocks until shutdown
```

---

## V1 Config (config.yaml)

```yaml
polling_interval: 5m

companies:
  - name: stripe
    ats: greenhouse
    board_token: "stripe"
    enabled: true

  - name: vercel
    ats: greenhouse
    board_token: "vercel"
    enabled: true

  - name: notion
    ats: greenhouse
    board_token: "notion"
    enabled: true

filters:
  title_keywords:
    - software engineer
    - backend
    - fullstack
    - sde
    - platform
    - infrastructure
  locations:
    - United States
    - US
    - Remote
```

---

## Freshness Detection Strategy

Two approaches used together:

1. **API timestamp**: If the API returns `posted_at`, filter for `posted_at > now - 1hr`
2. **First-seen tracking**: Any job ID not previously in our SQLite store is "new". `first_seen` is set to our clock time on first encounter.

Greenhouse provides `updated_at` on jobs, so V1 can use approach 1 primarily with approach 2 as a safety net.

---

## Filtering Pipeline

```
Raw Jobs from FetchJobs()
  → Title Filter (keyword match)
  → Location Filter (US / Remote)
  → Freshness Check (posted_at > now - 1hr OR first_seen = this poll)
  → Dedup Check (already notified? skip)
  → New Job Alert
```

---

## Project Structure

```
firstin/
├── cmd/
│   └── firstin/
│       └── main.go              # wiring, config loading, run
├── internal/
│   ├── config/
│   │   └── config.go            # YAML config parsing
│   ├── model/
│   │   └── job.go               # Job struct
│   ├── adapter/
│   │   └── greenhouse.go        # GreenhouseAdapter (implements JobFetcher)
│   ├── filter/
│   │   └── filter.go            # TitleAndLocationFilter (implements JobFilter)
│   ├── store/
│   │   └── sqlite.go            # SQLiteStore (implements JobStore)
│   ├── notifier/
│   │   └── log.go               # LogNotifier (implements Notifier)
│   ├── poller/
│   │   └── poller.go            # CompanyPoller (orchestrates one poll cycle)
│   └── scheduler/
│       └── scheduler.go         # Scheduler (main loop, timing, shutdown)
├── config.yaml
├── go.mod
├── go.sum
└── CLAUDE.md
```

---

## V2 Roadmap (Build Later)

Every upgrade below is additive — no existing code needs modification thanks to the interface-based design.

### Add a New ATS Adapter (e.g., Lever)

1. Create `internal/adapter/lever.go` implementing `JobFetcher`
2. In `main.go`, if `company.ATS == "lever"` → use `LeverAdapter`
3. Zero changes to CompanyPoller, Scheduler, Filter, Store

### Add Slack Notifications

1. Create `internal/notifier/slack.go` implementing `Notifier`
2. In `main.go`, swap `NewLogNotifier()` → `NewSlackNotifier(webhookURL)`
3. Or create `MultiNotifier` that fans out to both
4. Zero changes to CompanyPoller, Scheduler

### Add Circuit Breaking (Decorator Pattern)

1. Create `CircuitBreakerFetcher` that wraps any `JobFetcher`
2. It implements `JobFetcher` itself, tracks failures, short-circuits when OPEN
3. Wire in main.go: `fetcher = NewCircuitBreaker(adapter)`
4. Zero changes to CompanyPoller, Scheduler
5. States: CLOSED (normal) → 5 failures → OPEN (skip) → 30min cooldown → HALF-OPEN (try one) → success → CLOSED

### Add Retry with Exponential Backoff + Jitter

1. Create `RetryFetcher` that wraps any `JobFetcher`
2. On error, retries with backoff: 5s → 15s → give up
3. Jitter: add random ±30s to poll intervals to look organic and distribute load
4. Wire in main.go: `fetcher = NewRetryFetcher(adapter, maxRetries)`
5. Zero changes to CompanyPoller, Scheduler

### Add Worker Pool / Concurrency (Semaphore)

1. Scheduler.Run() changes from sequential `for` loop to `pool.Submit(poller.Poll)`
2. Semaphore caps concurrent polls (e.g., max 10 simultaneous)
3. Zero changes to CompanyPoller

### Add Priority Queue Scheduling

1. Scheduler.Run() changes from for-range + fixed sleep to pop-from-heap + sleep-until-next
2. Each company gets its own interval, reinserted after poll with `next_poll_at = now + interval + jitter`
3. Zero changes to CompanyPoller

### Add Adaptive Polling Intervals

1. If company frequently posts → keep interval tight (5m)
2. If company hasn't posted in days → gradually increase (5m → 10m → 15m)
3. If new job detected → temporarily shorten (companies post in batches)
4. Bounded: never faster than 3m, never slower than 30m

### Add Composite Filters

1. Create `CompositeFilter` implementing `JobFilter`, holds `[]JobFilter`
2. Wire: `filter = NewCompositeFilter(titleFilter, locationFilter, seniorityFilter)`
3. Zero changes to CompanyPoller

### Decorator Stacking (The Key Pattern)

The `JobFetcher` interface wraps itself, letting you stack behaviors like middleware:

```
V1:  fetcher = GreenhouseAdapter
V2:  fetcher = RetryFetcher → GreenhouseAdapter
V3:  fetcher = CircuitBreaker → RetryFetcher → GreenhouseAdapter
V4:  fetcher = RateLimiter → CircuitBreaker → RetryFetcher → GreenhouseAdapter
```

CompanyPoller always sees just a `JobFetcher`. It calls `FetchJobs()`. It has no idea how many layers wrap the real adapter.

---

## Retry Strategy Reference (V2)

```
HTTP 429       → read Retry-After header, wait, retry once
5xx/timeout    → retry up to 2x with backoff (5s → 15s → give up)
Parse error    → do NOT retry (retrying won't help), log raw response
403/captcha    → do NOT retry, flag for manual review
```

---

## Adding a New Company (Decision Tree)

```
Want to add company X?
  → What ATS do they use? (check careers page network tab)
    → Greenhouse: add 1 line to config with board_token
    → Lever: add 1 line to config with slug (V2)
    → Ashby: add 1 line to config with org (V2)
    → Unknown/Custom: write adapter (~50-100 lines) implementing JobFetcher (V2)
```

---

## Coding Conventions

- Use `slog` for structured logging
- Use `context.Context` for cancellation throughout
- All interfaces live close to the consumer, not the implementer (Go idiom)
- Error handling: wrap errors with `fmt.Errorf("doing X: %w", err)` for traceability
- No global state — everything injected via constructors in `main()`
- Tests: use interface mocks, no real HTTP calls in unit tests
- Tests: only the most important tests per component — max 5 tests each. No over-testing. Focus on happy path, key error paths, and critical edge cases.
- Task runner: use a `justfile` (https://github.com/casey/just) for all project commands (build, test, run, lint, etc.) — not Makefile