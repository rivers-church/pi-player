# Remote Administration

This document covers common remote administration tasks for pi-player kiosk systems.

## Using hyprctl Over SSH

Hyprland's `hyprctl` command requires the `HYPRLAND_INSTANCE_SIGNATURE` environment variable to identify which Hyprland instance to control. When connecting over SSH, this variable is not automatically set.

### Quick Command

```bash
# Single command
ssh user@target 'export HYPRLAND_INSTANCE_SIGNATURE=$(ls -1 /run/user/$(id -u)/hypr/ | head -n 1) && hyprctl reload'
```

### Setting Up an Alias

For convenience, add this to the **target machine's** `~/.bashrc`:

```bash
# Add this to ~/.bashrc on the target machine
alias hyprctl-auto='export HYPRLAND_INSTANCE_SIGNATURE=$(ls -1 /run/user/$(id -u)/hypr/ | head -n 1) && hyprctl'
```

Then use it over SSH:

```bash
ssh user@target 'hyprctl-auto reload'
ssh user@target 'hyprctl-auto clients'
ssh user@target 'hyprctl-auto dispatch exit'
```

### Common hyprctl Commands

```bash
# Reload Hyprland configuration
hyprctl-auto reload

# List all windows/clients
hyprctl-auto clients

# Kill a specific window
hyprctl-auto dispatch killactive

# Exit Hyprland (will restart if configured via .bash_profile)
hyprctl-auto dispatch exit

# Get version info
hyprctl-auto version
```

## Controlling Pi-Player Service

```bash
# Start pi-player
ssh user@target 'systemctl --user start pi-player'

# Stop pi-player
ssh user@target 'systemctl --user stop pi-player'

# Restart pi-player
ssh user@target 'systemctl --user restart pi-player'

# Check status
ssh user@target 'systemctl --user status pi-player'

# View logs (last 50 lines)
ssh user@target 'journalctl --user -u pi-player -n 50'

# Follow logs in real-time
ssh user@target 'journalctl --user -u pi-player -f'
```

## Testing Volume OSD

```bash
# Manually trigger volume up OSD
ssh user@target 'WAYLAND_DISPLAY=wayland-1 swayosd-client --output-volume raise'

# Manually trigger volume down OSD
ssh user@target 'WAYLAND_DISPLAY=wayland-1 swayosd-client --output-volume lower'

# Toggle mute
ssh user@target 'WAYLAND_DISPLAY=wayland-1 swayosd-client --output-volume mute-toggle'
```

## Audio Control

```bash
# Get current volume
ssh user@target 'wpctl get-volume @DEFAULT_AUDIO_SINK@'

# Set volume to 50%
ssh user@target 'wpctl set-volume @DEFAULT_AUDIO_SINK@ 50%'

# Increase volume by 5%
ssh user@target 'wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%+'

# Decrease volume by 5%
ssh user@target 'wpctl set-volume @DEFAULT_AUDIO_SINK@ 5%-'

# Mute
ssh user@target 'wpctl set-mute @DEFAULT_AUDIO_SINK@ 1'

# Unmute
ssh user@target 'wpctl set-mute @DEFAULT_AUDIO_SINK@ 0'

# Toggle mute
ssh user@target 'wpctl set-mute @DEFAULT_AUDIO_SINK@ toggle'
```

## Network Configuration

```bash
# Check DNS resolution status
ssh user@target 'resolvectl status'

# Test DNS resolution
ssh user@target 'resolvectl query hostname.example.com'

# Check network interfaces
ssh user@target 'networkctl status'

# Restart networking
ssh user@target 'sudo systemctl restart systemd-networkd systemd-resolved'
```

## System Information

```bash
# Check system uptime
ssh user@target 'uptime'

# Check disk usage
ssh user@target 'df -h'

# Check memory usage
ssh user@target 'free -h'

# Check running processes
ssh user@target 'ps aux | grep -E "(Hyprland|pi-player|swayosd)"'

# Check system logs
ssh user@target 'journalctl -b -n 50'
```

## Rebooting and Power Management

```bash
# Reboot the system
ssh user@target 'sudo reboot'

# Shutdown the system
ssh user@target 'sudo poweroff'

# Exit Hyprland (will auto-restart if configured)
ssh user@target 'export HYPRLAND_INSTANCE_SIGNATURE=$(ls -1 /run/user/$(id -u)/hypr/ | head -n 1) && hyprctl dispatch exit'
```

## Environment Variables for Wayland Applications

When running GUI applications over SSH, you may need these environment variables:

```bash
export WAYLAND_DISPLAY=wayland-1
export XDG_RUNTIME_DIR=/run/user/$(id -u)
```

Example:
```bash
ssh user@target 'export WAYLAND_DISPLAY=wayland-1 && swayosd-client --output-volume raise'
```

## Tips for SSH Sessions

### Using SSH Config

Add to your local `~/.ssh/config`:

```
Host pi-player-*
    User sandtonvisuals
    ForwardAgent yes
    ServerAliveInterval 60

Host pi-player-aud1
    HostName 192.168.20.80
```

Then connect with:
```bash
ssh pi-player-aud1
```

### Persistent Sessions with tmux/screen

```bash
# Start a persistent session
ssh user@target 'tmux new -s admin'

# Reattach to session
ssh user@target 'tmux attach -t admin'
```
