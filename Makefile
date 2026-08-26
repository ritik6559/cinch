MODULE := github.com/ritik6559/cinch
BINARY := bin/cinch$(shell go env GOEXE)

# Windows make picks cmd.exe unless it can find a POSIX shell on PATH. From Git
# Bash it finds one; from PowerShell it does not, and every /dev/null and `test`
# below fails. Git Bash is already required for the bash tool, so name it.
ifeq ($(OS),Windows_NT)
  SHELL := bash.exe
  .SHELLFLAGS := -c
endif

# ?= means "only if not already set", so you can override from the command line:
#   make build VERSION=1.0.0
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# -X writes a value into a string variable at link time. The path must be the
# full package path, or the linker silently does nothing.
# -s and -w remove the symbol table and debug data, making the binary smaller.
LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

# .PHONY lists targets that are commands, not files. Without it, a file named
# "test" in this directory would stop `make test` from running.
.PHONY: build run install test vet fmt tidy check clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/cinch

run:
	go run ./cmd/cinch

# Installs into $(go env GOPATH)/bin so you can type `cinch` anywhere.
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/cinch

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

# Everything CI runs, in one command. Run this before you push.
check: vet test
	@test -z "$$(gofmt -l .)" || (echo "run: make fmt" && gofmt -l . && exit 1)
	go build ./...

clean:
	rm -rf bin
