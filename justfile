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

# Build release snapshot locally without publishing (requires goreleaser)
release-dry:
    goreleaser release --snapshot --clean

# Tag and push to trigger a GitHub Release via GoReleaser (first run release-dry)
release version:
    git tag {{version}}
    git push origin {{version}}
    goreleaser release --clean

