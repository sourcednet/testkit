GO ?= go

.PHONY: all build test lint fmt
all: lint test build

build:
	$(GO) build ./...

test:
	$(GO) test -race ./...

# gofmt and go vet. staticcheck isn't downloaded yet: ask before adding it.
lint:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)
	$(GO) vet ./...

fmt:
	gofmt -w .

# Local Docker test network (needs Docker). See docker/README.md.
testnet:
	docker/prepare.sh

testnet-up: testnet
	docker compose -f docker/docker-compose.yml up -d

testnet-down:
	docker compose -f docker/docker-compose.yml down
