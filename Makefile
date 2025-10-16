.PHONY: build test run debug-tunnel debug-remote-start debug-local help

# Build the pi-player binary
build:
	go build -v -o pi-player main.go

# Run tests
test:
	go test -v ./...

# Run locally
run:
	go run main.go

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
	@echo "  make build               - Build the pi-player binary"
	@echo "  make test                - Run tests"
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
