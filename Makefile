GO ?= go
GOFMT ?= gofmt

.PHONY: build fmt-check vet test race check clean release-binary release-repro

# Reproducible release binary for the signed Curator distribution
# (FRONT-DOOR §10). Curator's release workflow calls it per platform, then
# signs, notarizes and packs; none of that happens here.
#   make release-binary VERSION=0.6.0 [GOOS=darwin GOARCH=arm64] [DIST=dist]
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
DIST ?= dist
RELEASE_TOOLCHAIN := go$(shell sed -n 's/^go //p' go.mod)
RELEASE_NAME = curator-run-$(VERSION)-$(GOOS)-$(GOARCH)

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

# VERSION goes into the ldflags, so it must be a plain semantic version.
# The toolchain is pinned to go.mod, and -buildid= keeps the bytes the same
# across hosts. The checksum is in sha256sum format, so `sha256sum -c` (or
# `shasum -a 256 -c`) verifies it.
release-binary:
	@printf '%s\n' "$(VERSION)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$$' || \
		{ echo "release-binary: VERSION must be a semantic version, got '$(VERSION)'" >&2; exit 2; }
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) GOFLAGS=-mod=readonly GOTOOLCHAIN=$(RELEASE_TOOLCHAIN) \
		$(GO) build -trimpath -buildvcs=false \
		-ldflags "-s -w -buildid= -X main.buildVersion=$(VERSION)" \
		-o $(DIST)/$(RELEASE_NAME) ./cmd/curator-run
	@cd $(DIST) && if command -v sha256sum >/dev/null 2>&1; then sha256sum $(RELEASE_NAME); \
		else shasum -a 256 $(RELEASE_NAME); fi > $(RELEASE_NAME).sha256 && cat $(RELEASE_NAME).sha256

# Builds the committed HEAD in two separate checkout directories and requires
# identical bytes.
release-repro:
	scripts/release-repro.sh
