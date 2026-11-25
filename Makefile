.PHONY: frontend build test test-race vulncheck check release-check release-build release-version release run debug-tunnel debug-remote-start debug-local help

# Bundle the frontend. go:embed bakes the result into the binary at compile
# time, so this has to happen before `go build` or the pages load nothing.
frontend:
	deno task build

# Build the pi-player binary
build: frontend
	go build -v -o pi-player main.go

# Frontend gates: types, lint and unit tests.
check:
	deno task check
	deno task lint
	deno task test

# Run tests
test:
	go test -v ./...

# Run tests with the race detector. Most of the concurrency the player does -
# websockets, the directory watcher, overlapping API calls - is only checked
# under -race, so run this before shipping.
test-race:
	go test -race -count=2 ./...

# Fail if a reachable known vulnerability affects the build (CI runs the same).
# Pinned: govulncheck v1.1.x crashes on Go 1.27.
GOVULNCHECK_VERSION ?= v1.9.0
vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	govulncheck ./...

# Run the release gates in order, even when make is invoked with -j.
release-check:
	$(MAKE) frontend
	$(MAKE) check
	@unformatted=$$(gofmt -l .) || exit 1; \
	if [ -n "$$unformatted" ]; then \
		printf 'These files need gofmt:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	$(MAKE) test
	$(MAKE) test-race
	$(MAKE) vulncheck

export VERSION CONFIRM DRY_RUN

release-version:
	@if [ -z "$$VERSION" ]; then \
		echo 'Error: VERSION is required (for example VERSION=v0.4.1).'; \
		exit 1; \
	fi; \
	case "$$VERSION" in \
		*[!v0-9.rc-]*) valid=no ;; \
		*) if printf '%s\n' "$$VERSION" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$$'; then valid=yes; else valid=no; fi ;; \
	esac; \
	if [ "$$valid" != yes ]; then \
		echo 'Error: VERSION must be vX.Y.Z or vX.Y.Z-rcN (numeric X, Y, Z and N).'; \
		exit 1; \
	fi

# Match the former Ubuntu runner's Linux x86-64 artifact for provisioning.
release-build: release-version
	$(MAKE) frontend
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$$VERSION" -o pi-player

# Use MAKE_COMMAND so GNU Make's -n never executes this publishing recipe.
release: release-version
	@set -eu; \
	fail() { printf 'Error: %s\n' "$$1" >&2; exit 1; }; \
	if [ "$$CONFIRM" != 1 ]; then \
		fail "CONFIRM=1 is required: this publishes the binary that every provisioned device downloads as 'latest' (except for prereleases)."; \
	fi; \
	status=$$(git status --porcelain --untracked-files=all); \
	[ -z "$$status" ] || fail 'The working tree must be clean.'; \
	branch=$$(git symbolic-ref --quiet --short HEAD) || fail 'Releases must be run from master.'; \
	[ "$$branch" = master ] || fail 'Releases must be run from master.'; \
	head=$$(git rev-parse --verify refs/heads/master); \
	upstream=$$(git rev-parse --verify refs/remotes/origin/master 2>/dev/null) || fail 'origin/master is missing; fetch origin first.'; \
	[ "$$head" = "$$upstream" ] || fail 'master must match origin/master.'; \
	tag="refs/tags/$$VERSION"; \
	if git show-ref --verify --quiet "$$tag"; then fail "Tag $$VERSION already exists locally."; fi; \
	remote=$$(git ls-remote --refs origin refs/heads/master "$$tag") || fail 'Could not verify origin/master and the tag on origin.'; \
	remote_head=$$(printf '%s\n' "$$remote" | awk '$$2 == "refs/heads/master" { print $$1 }'); \
	[ "$$head" = "$$remote_head" ] || fail 'master must match the current master on origin; fetch origin first.'; \
	remote_tag=$$(printf '%s\n' "$$remote" | awk -v tag="$$tag" '$$2 == tag { print $$1 }'); \
	[ -z "$$remote_tag" ] || fail "Tag $$VERSION already exists on origin."; \
	run() { \
		if [ "$$DRY_RUN" = 1 ]; then \
			printf '%s' "$$1"; shift; printf ' %s' "$$@"; printf '\n'; \
		else "$$@"; fi; \
	}; \
	case "$$VERSION" in *-rc*) set -- --prerelease ;; *) set -- ;; esac; \
	run $(MAKE_COMMAND) release-check; \
	run $(MAKE_COMMAND) release-build "VERSION=$$VERSION"; \
	status=$$(git status --porcelain --untracked-files=all); \
	[ -z "$$status" ] && [ "$$(git rev-parse HEAD)" = "$$head" ] || fail 'The checkout changed while preparing the release.'; \
	run git tag -a "$$VERSION" -m "$$VERSION" "$$head"; \
	run git push --no-follow-tags origin "$$tag"; \
	run gh release create "$$VERSION" ./pi-player --title "$$VERSION" --generate-notes --verify-tag --repo rivers-church/pi-player "$$@"

# Run locally
run:
	go run main.go


dev: frontend
	go run main.go --debug --test web
# Set up SSH tunnel for remote debugging
# Usage: make debug-tunnel HOST=user@host
# Or:    export PI_PLAYER_DEBUG_HOST=user@host && make debug-tunnel
debug-tunnel:
	@./scripts/debug-remote.sh $(HOST)

# Start remote debugging session (SSH to remote and start delve)
# Usage: make debug-remote-start HOST=user@host
# Or:    export PI_PLAYER_DEBUG_HOST=user@host && make debug-remote-start
debug-remote-start:
	@TARGET="$(if $(HOST),$(HOST),$(PI_PLAYER_DEBUG_HOST))"; \
	if [ -z "$$TARGET" ]; then \
		echo "Error: HOST not specified"; \
		echo "Usage: make debug-remote-start HOST=user@host"; \
		echo "   Or: export PI_PLAYER_DEBUG_HOST=user@host"; \
		exit 1; \
	fi; \
	echo "Starting remote debug session on $$TARGET..."; \
	echo "Remember to run 'make debug-tunnel HOST=$$TARGET' in another terminal first!"; \
	echo ""; \
	ssh -t $$TARGET "systemctl --user stop pi-player && export WAYLAND_DISPLAY=wayland-1 && dlv exec ~/.local/bin/pi-player --headless --listen=:2345 --api-version=2 --accept-multiclient"

# Start local debugging with delve
debug-local:
	dlv debug main.go --headless --listen=:2345 --api-version=2 --accept-multiclient

# Show help
help:
	@echo "Pi-Player Makefile Commands:"
	@echo ""
	@echo "  make build               - Bundle the frontend, then build the binary"
	@echo "  make frontend            - Bundle the frontend only"
	@echo "  make check               - Frontend type-check, lint and tests"
	@echo "  make test                - Run tests"
	@echo "  make test-race           - Run tests with the race detector"
	@echo "  make run                 - Run locally"
	@echo "  make debug-local         - Start local debugging server (delve)"
	@echo "  make debug-tunnel        - SSH tunnel for remote debugging"
	@echo "                             Usage: make debug-tunnel HOST=user@host"
	@echo "  make debug-remote-start  - Start delve on remote machine"
	@echo "                             Usage: make debug-remote-start HOST=user@host"
	@echo "                             Or set PI_PLAYER_DEBUG_HOST env variable"
	@echo ""
	@echo "Remote debugging workflow:"
	@echo "  1. Terminal 1: make debug-tunnel HOST=user@192.168.20.80"
	@echo "  2. Terminal 2: make debug-remote-start HOST=user@192.168.20.80"
	@echo "  3. In nvim: :DapContinue (or VSCode: F5)"
