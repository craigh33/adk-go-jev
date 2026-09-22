.PHONY: pre-commit-install pre-commit test test-live build lint check-examples generate check-generated

# Install pre-commit hooks (requires pre-commit to be installed).
pre-commit-install:
	$(MAKE) pre-commit

# Install pre-commit hooks (requires pre-commit to be installed).
pre-commit:
	@command -v pre-commit >/dev/null 2>&1 || brew install pre-commit
	pre-commit install --install-hooks -t pre-commit -t commit-msg


# Run unit tests for all packages.
test:
	go test ./... -count=1

# Opt-in calls to the real API; requires TYPESAFE_API_KEY.
test-live:
	go test -tags=integration ./typesafe -run '^TestLiveSystemOne$$' -count=1 -v

check-examples:
	$(MAKE) -C examples/bedrock-routing test build lint

# Compile all packages (no test run).
build:
	go build ./...

# Run golangci-lint (see .golangci.yaml).
lint:
	golangci-lint run ./...

generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config api/oapi-codegen.yaml https://api.typesafe.ai/openapi.json

check-generated: generate
	git diff --exit-code -- internal/typesafe/types.gen.go
