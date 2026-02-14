# FirstIn

FirstIn polls company career pages for newly posted software engineering roles and alerts you before anyone else sees them. It checks direct career APIs on a schedule, filters by title and location keywords, deduplicates with a local SQLite store, and logs new matches to stdout.

V1 supports Greenhouse boards only.

## Quick Start

```
just build
just run
```

## Configuration

Edit `config.yaml`:

```yaml
polling_interval: 15m

companies:
  - name: stripe
    ats: greenhouse
    board_token: "stripe"
    enabled: true

filters:
  title_keywords:
    - software engineer
    - backend
  locations:
    - United States
    - Remote
```

To add a new Greenhouse company, add one entry under `companies` with the board token (visible in the careers page URL).

## Commands

```
just              # list all commands
just build        # compile binary
just run          # run with default config
just run-debug    # run with debug logging
just dry-run      # poll once, print matches, don't persist
just test         # run tests
just lint         # vet
just clean        # remove binary and db
```
