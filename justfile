# FirstIn — task runner (just)

build:
    go build -o firstin ./cmd/firstin/

test:
    go test ./...

run:
    go run ./cmd/firstin/

run-debug:
    go run ./cmd/firstin/ -debug

lint:
    go vet ./...

clean:
    rm -f firstin jobs.db
