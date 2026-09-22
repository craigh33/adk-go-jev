.PHONY: run test build lint

run:
	go run .

test:
	go test ./...

build:
	go build ./...

lint:
	golangci-lint run --config ../../.golangci.yaml ./...
