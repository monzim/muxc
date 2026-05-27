VERSION ?= dev

.PHONY: build install test test-int coverage lint clean

build:
	CGO_ENABLED=0 go build \
		-ldflags "-s -w \
			-X main.Version=$(VERSION) \
			-X main.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown) \
			-X main.Date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)" \
		-trimpath \
		-o bin/muxc \
		./cmd/muxc

install: build
	cp bin/muxc ~/.local/bin/muxc

test:
	go test ./...

test-int:
	go test -tags integration ./...

# coverage merges unit + integration coverage.
# Integration tests spawn the muxc binary as a subprocess; we instrument it
# with `go build -cover` and merge GOCOVERDIR data with the unit profile.
coverage:
	@mkdir -p bin coverage/int
	@go test -coverprofile=coverage/unit.out -covermode=set ./internal/... > /dev/null
	@CGO_ENABLED=0 go build -cover -covermode=set -o bin/muxc-cov ./cmd/muxc
	@GOCOVERDIR=$(PWD)/coverage/int MUXC_COV_BIN=$(PWD)/bin/muxc-cov \
		go test -tags integration ./internal/cli > /dev/null || true
	@go tool covdata textfmt -i=coverage/int -o=coverage/int.out 2>/dev/null || true
	@echo "mode: set" > coverage/merged.out
	@tail -n +2 coverage/unit.out >> coverage/merged.out
	@if [ -s coverage/int.out ]; then tail -n +2 coverage/int.out >> coverage/merged.out; fi
	@go tool cover -func=coverage/merged.out | tail -1

lint:
	golangci-lint run

clean:
	rm -rf bin/
