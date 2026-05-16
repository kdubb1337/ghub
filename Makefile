.PHONY: build fmt lint test test-integration ci tools install clean

CLI_NAME := ghub
MODULE   := $(shell go list -m)

build:
	@mkdir -p bin
	go build -o bin/$(CLI_NAME) ./cmd/$(CLI_NAME)

install: build
	cp bin/$(CLI_NAME) $(GOPATH)/bin/$(CLI_NAME)

fmt:
	gofumpt -l -w .
	goimports -local $(MODULE) -w .

lint:
	golangci-lint run ./...
	./scripts/lint-naming.sh

test:
	go test ./...

test-integration:
	go test -tags=integration ./internal/integration/...

ci: fmt lint test build

tools:
	GOBIN=$(CURDIR)/.tools go install mvdan.cc/gofumpt@latest
	GOBIN=$(CURDIR)/.tools go install golang.org/x/tools/cmd/goimports@latest
	GOBIN=$(CURDIR)/.tools go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

clean:
	rm -rf bin .tools
