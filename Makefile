GO ?= go
GOFMT ?= gofmt

.PHONY: build fmt-check vet test race verify check clean release-binary release-repro release-gate-test

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

# cmd/curator-run builds and drives the real binary; on a loaded host it has
# run close to go test's default 10m per-package limit. 13m stays inside the
# hosted test job's 15-minute budget, so a slow package still fails with a Go
# timeout stack rather than a killed job.
# Conformance includes deliberate deadline controls; leave race-runner margin.
# This per-package bound remains below the hosted race job's 15-minute limit.
# Each selected lane runs even after another fails; the collector reports all
# verdicts and returns nonzero for any failure or unavailable tool. verify is
# the hosted non-race selection; check also includes the race lanes.
# Preserve target values as data while go run builds a host-native collector.
export VALIDATE_GOOS := $(value GOOS)
export VALIDATE_GOARCH := $(value GOARCH)
export VALIDATE_GOOS_SET := $(if $(filter undefined,$(origin GOOS)),,1)
export VALIDATE_GOARCH_SET := $(if $(filter undefined,$(origin GOARCH)),,1)
build fmt-check vet test race verify check:
	GOOS=$$($(GO) env GOHOSTOS) GOARCH=$$($(GO) env GOHOSTARCH) $(GO) run ./scripts/validate -target $@ -go '$(GO)' -gofmt '$(GOFMT)' -target-env

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
