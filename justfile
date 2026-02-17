# FirstIn — task runner (just)

default:
    @just --list

build:
    go build -o firstin ./cmd/firstin/

test:
    go test ./...

run:
    go run ./cmd/firstin/

run-debug:
    go run ./cmd/firstin/ -debug

dry-run:
    go run ./cmd/firstin/ -dry-run

test-slack:
    go run ./cmd/firstin/ -test-slack

audit:
    go run ./cmd/firstin/ -audit

lint:
    go vet ./...

clean:
    rm -f firstin jobs.db
