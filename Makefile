GO ?= go
GOFMT ?= gofmt

.PHONY: build fmt-check vet test race check clean

build:
	$(GO) build ./...

# Fails when any Go file is not gofmt-clean; prints the offenders.
fmt-check:
	@unformatted="$$($(GOFMT) -l cmd internal schemas)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: files need formatting:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./... -count=1

# Conformance includes deliberate deadline controls; leave race-runner margin.
# This per-package bound remains below the hosted race job's 15-minute limit.
race:
	$(GO) test ./... -count=1 -race -timeout=12m

check: build fmt-check vet test race

clean:
	$(GO) clean ./...
