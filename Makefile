GO ?= go

.PHONY: build vet test check clean

build:
	$(GO) build ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./... -count=1

check: build vet test

clean:
	$(GO) clean ./...
