# Debugging Pi-Player

This project uses VSCode's `launch.json` format for debug configurations, making it compatible with both VSCode and Neovim (via nvim-dap).

## Quick Start

### Remote Debugging (Recommended)

**Option A: Using Make (Easiest)**

1. **Terminal 1** - Start SSH tunnel:
   ```bash
   make debug-tunnel HOST=user@target-machine
   ```

2. **Terminal 2** - Start delve on remote:
   ```bash
   make debug-remote-start HOST=user@target-machine
   ```

**Option B: Manual**

1. **Start SSH tunnel** (in terminal 1):
   ```bash
   make debug-tunnel HOST=user@target-machine
   ```

2. **SSH to remote** (in terminal 2) and run:
   ```bash
   ssh user@target-machine
   systemctl --user stop pi-player
   export WAYLAND_DISPLAY=wayland-1
   dlv exec ~/.local/bin/pi-player --headless --listen=:2345 --api-version=2 --accept-multiclient
   ```

   **Note:** The `WAYLAND_DISPLAY` export is required when running over SSH to connect to the graphical session.

3. **In your editor**:

   **Neovim:**
   - Open the pi-player project
   - Press `<leader>dc` (or `:DapContinue`)
   - Select "Attach to Remote (SSH tunnel)"

   **VSCode:**
   - Open the pi-player project
   - Go to Run and Debug (Ctrl+Shift+D)
   - Select "Attach to Remote (SSH tunnel)" from dropdown
   - Press F5

### Local Debugging

1. **Start delve**:
   ```bash
   make debug-local
   ```

2. **In your editor**:

   **Neovim:**
   - Press `<leader>dc`
   - Select "Debug pi-player (local)"

   **VSCode:**
   - Go to Run and Debug (Ctrl+Shift+D)
   - Select "Debug pi-player (local)"
   - Press F5

## Debugging Approaches

### 1. SSH Port Forwarding (Used Here)

**Pros:**
- Works across any network
- No firewall configuration needed
- Secure (uses SSH)
- Simple setup

**Cons:**
- Requires SSH access
- Extra terminal for tunnel

**How it works:**
```
[Neovim/DAP] -> localhost:2345 -> [SSH Tunnel] -> remote:2345 [Delve]
```

### 2. Direct Connection

Connect directly to delve on the remote machine.

**Setup:** Modify `.nvim.lua`:
```lua
host = '192.168.20.80',  -- Use actual remote IP
port = 2345,
```

**Pros:**
- No SSH tunnel needed
- Slightly faster

**Cons:**
- Firewall must allow port 2345
- Less secure
- Hardcoded IPs

### 3. Remote Development (SSH + nvim)

SSH to remote machine and run nvim there with local debugging.

**Pros:**
- Simplest setup
- No port forwarding

**Cons:**
- Need nvim config on remote
- Terminal-based only

### 4. VSCode launch.json (Used by This Project)

This project uses `.vscode/launch.json` for debug configurations, which works in both VSCode and Neovim.

**Benefits:**
- Single source of truth for debug configs
- Cross-editor compatibility
- Standard JSON format
- Can be edited visually in VSCode

Neovim loads these configs via `require('dap.ext.vscode').load_launchjs()` in `.nvim.lua`.

## Configuration Files

- `.vscode/launch.json` - Debug configurations (works in both VSCode and Neovim)
- `.nvim.lua` - Loads launch.json for nvim-dap (auto-loaded by LazyVim)
- `Makefile` - Helper commands for building and debugging
- `scripts/debug-remote.sh` - SSH tunnel helper script

## Environment Variables

- `PI_PLAYER_DEBUG_HOST` - Default remote host for debugging
  ```bash
  export PI_PLAYER_DEBUG_HOST=sandtonvisuals@192.168.20.80
  ```

## Troubleshooting

### "Connection refused" when attaching
- Verify SSH tunnel is running: `lsof -i :2345`
- Check delve is listening on remote: `ss -tlnp | grep 2345`

### "Failed to connect to Wayland display" when running over SSH
- Ensure you set `WAYLAND_DISPLAY=wayland-1` before running delve
- Or use `make debug-remote-start` which sets it automatically

### Breakpoints not hitting
- Ensure source paths match: check `remotePath` in `.vscode/launch.json`
- Rebuild with debug symbols: `go build -gcflags="all=-N -l"`

### Different project paths on remote
Update `.vscode/launch.json`:
```json
{
  "name": "Attach to Remote (SSH tunnel)",
  "remotePath": "/remote/path/to/pi-player",
  ...
}
```
