GO ?= go
GOFMT ?= gofmt

.PHONY: build fmt-check vet test race check clean release-binary release-repro release-gate-test

# Reproducible release binary for the signed Curator distribution
# (FRONT-DOOR §10). Curator's release workflow calls it per platform, then
# signs, notarizes and packs. The launcher's optional GitHub artifact workflow
# does the same after the build; neither signing nor publishing happens here.
#   make release-binary VERSION=0.6.0 [GOOS=darwin GOARCH=arm64] [DIST=dist]
# The inputs are data. $(value ...) takes their text unexpanded, and the raw
# VERSION and DIST are never exported. So a crafted value never runs in make
# or in the shell, and scripts/release-binary.sh validates it before use.
export RELEASE_VERSION := $(value VERSION)
export RELEASE_GOOS := $(value GOOS)
export RELEASE_GOARCH := $(value GOARCH)
export RELEASE_DIST := $(value DIST)
unexport VERSION DIST
# GNU make expands a command-line variable when it exports it. For the
# release goals, a command-line GOOS or GOARCH is therefore passed only as
# data. Other targets keep the usual GOOS/GOARCH passthrough.
ifneq ($(filter release-binary release-repro,$(MAKECMDGOALS)),)
ifeq ($(origin GOOS),command line)
unexport GOOS
endif
ifeq ($(origin GOARCH),command line)
unexport GOARCH
endif
endif

build:
	$(GO) build ./...
	$(GO) -C schemas/session-manager build ./...

# Fails when any Go file is not gofmt-clean; prints the offenders.
fmt-check:
	@unformatted="$$($(GOFMT) -l cmd internal schemas)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: files need formatting:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	$(GO) vet ./...
	$(GO) -C schemas/session-manager vet ./...

# cmd/curator-run builds and drives the real binary; on a loaded host it has
# run close to go test's default 10m per-package limit. 13m stays inside the
# hosted test job's 15-minute budget, so a slow package still fails with a Go
# timeout stack rather than a killed job.
test:
	$(GO) test ./... -count=1 -timeout=13m
	$(GO) -C schemas/session-manager test ./... -count=1

# Conformance includes deliberate deadline controls; leave race-runner margin.
# This per-package bound remains below the hosted race job's 15-minute limit.
race:
	$(GO) test ./... -count=1 -race -timeout=12m
	$(GO) -C schemas/session-manager test ./... -count=1 -race -timeout=12m

check: build fmt-check vet test race release-gate-test

clean:
	$(GO) clean ./...

release-binary:
	@scripts/release-binary.sh

# Builds the committed HEAD in two separate checkout directories and requires
# identical bytes.
release-repro:
	scripts/release-repro.sh

# Offline signed-tag admission and all-or-nothing signing configuration controls.
release-gate-test:
	@mkdir -p .temp
	scripts/release-gate-test.sh
