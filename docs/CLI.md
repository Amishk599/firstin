# FirstIn CLI

## Installation

```sh
# Install to $GOPATH/bin (run from anywhere after this)
go install ./cmd/firstin/

# Or build locally
just build      # produces ./firstin
```

---

## Global Flags

These flags are available on every subcommand.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--config` | `-c` | see below | Path to config file |
| `--debug` | | `false` | Enable debug logging |

### Config Discovery Order

1. `--config <path>` flag
2. `FIRSTIN_CONFIG` environment variable
3. `./config.yaml` (current directory)

---

## Commands

### `firstin start`

Start the polling daemon. Blocks until `SIGINT`/`SIGTERM`.

```sh
firstin start
firstin start --debug
firstin start --config /etc/firstin/config.yaml
```

### `firstin check`

One-shot poll. Fetches one company per ATS type, prints matched jobs, then exits. Does **not** write to the store — safe to run anytime without side effects.

```sh
firstin check
firstin check --debug
```

### `firstin audit`

Interactive TUI. Shows a company picker (with ASCII art header), then opens the split-pane audit view to browse all jobs vs. filtered matches.

```sh
firstin audit
firstin audit --config /path/to/config.yaml
```

Keybindings in the picker:

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `enter` | Select company |
| `q` | Quit |

### `firstin companies`

Print a table of all configured companies. No network calls — reads config only.

```sh
firstin companies
```

Example output:

```
Company                   ATS             Status
───────────────────────────────────────────────
anthropic                 greenhouse      enabled
chime                     greenhouse      enabled
nvidia                    workday         enabled
...

Total: 32 companies (32 enabled, 0 disabled)
```

### `firstin notify test`

Send a test notification through the configured notifier to verify the integration.

```sh
firstin notify test
```

> Requires `notification.type: slack` and a valid `webhook_url` in your config.

### `firstin version`

Print the current version string.

```sh
firstin version
# firstin 0.1.0
```

---

## Examples

```sh
# Run the daemon with a custom config
firstin start --config ~/configs/firstin.yaml

# Quick sanity check — see what would match right now
firstin check --debug

# Browse jobs for a specific company interactively
firstin audit

# List all companies in the config
firstin companies

# Verify your Slack webhook is wired up correctly
firstin notify test

# Use an env var to set the config path globally
export FIRSTIN_CONFIG=~/configs/firstin.yaml
firstin companies
firstin start
```

---

## Development Shortcuts (`just`)

```sh
just run          # go run ./cmd/firstin/ start
just run-debug    # go run ./cmd/firstin/ start --debug
just dry-run      # go run ./cmd/firstin/ check
just audit        # go run ./cmd/firstin/ audit
just test-slack   # go run ./cmd/firstin/ notify test
just build        # build ./firstin binary
just install      # go install ./cmd/firstin/
just test         # go test ./...
just lint         # go vet ./...
just clean        # remove binary and jobs.db
```
