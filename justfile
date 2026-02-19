# FirstIn — task runner (just)

default:
    @just --list

# Install binary to $GOPATH/bin (run from anywhere after this)
install:
    go install ./cmd/firstin/

build:
    go build -o firstin ./cmd/firstin/

test:
    go test ./...

# Dev shortcuts — use go run for fast iteration during development
run:
    go run ./cmd/firstin/ start

run-debug:
    go run ./cmd/firstin/ start --debug

dry-run:
    go run ./cmd/firstin/ check

test-slack:
    go run ./cmd/firstin/ notify test

audit:
    go run ./cmd/firstin/ audit

lint:
    go vet ./...

clean:
    rm -f firstin jobs.db
