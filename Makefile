BIN     := helikopter
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build install test lint fly clean preview

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/helikopter

install:
	CGO_ENABLED=0 go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/helikopter

test:
	go test ./... -race

lint:
	gofmt -l . | tee /dev/stderr | (! read)
	go vet ./...

bench:
	go test ./internal/render -bench . -benchmem -run '^$$'

# Write one PNG per theme so the art can be reviewed as pixels.
# The path is absolute because `go test` runs in the package directory, so a
# relative one lands inside internal/render rather than here.
preview:
	HELIKOPTER_PREVIEW=$(abspath $(or $(OUT),preview)) go test ./internal/render -run Preview -count=1

fly: build
	./$(BIN)

clean:
	rm -rf $(BIN) dist build preview
