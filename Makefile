VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= collect-bot:dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test lint run docker tidy clean

all: lint test build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/group-fund-bot ./cmd/bot

test:
	go test -race ./...

lint:
	go vet ./...
	golangci-lint run

run:
	go run ./cmd/bot

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

tidy:
	go mod tidy

clean:
	rm -rf bin dist
