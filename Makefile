.PHONY: run demo test check build
run:
	go run ./cmd/server

demo:
	@test -f data/trips.json || cp data/trips.example.json data/trips.json
	go run ./cmd/server

test:
	go test -race -cover ./...

check:
	@test -z "$$(gofmt -l cmd internal web/*.go)"
	go vet ./...
	go test -race ./...

build:
	CGO_ENABLED=0 go build -trimpath -o bin/diary ./cmd/server
