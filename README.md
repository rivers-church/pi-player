# Pi-Player

[![Go](https://github.com/rivers-church/pi-player/actions/workflows/build.yml/badge.svg)](https://github.com/rivers-church/pi-player/actions/workflows/build.yml)

A remotely controlled video and image player for Linux kiosk deployments. Runs on Arch Linux with Hyprland and exposes a web interface at `:8080` for control, playback, and settings.

---

## Provisioning a new device

Provisioning is a two-stage process: a scripted Arch install from the live ISO, followed by an automatic first-boot Ansible run that installs and configures pi-player.

### Stage 1 — Base install (from the live ISO)

Boot the target machine from an Arch Linux ISO **in UEFI mode**, connect it to ethernet (automatic), then run:

```bash
curl -fsSL https://raw.githubusercontent.com/rivers-church/pi-player/master/scripts/install.sh | bash
```

The installer prompts for everything that varies per device:

| Prompt | Notes |
|---|---|
| Hostname | e.g. `pi-lounge` |
| Username + password | Used for login, sudo, and SMB credentials default |
| Root password | |
| DHCP or static IP | Static prompts pre-fill current DHCP values as defaults |
| SMB network mount | Optional — prompts for share address, credentials, domain, mount point |
| Target disk | Interactive list — all data on selected disk will be erased |

Auto-detected from your IP (no prompts needed): timezone, locale, console keymap, and fastest pacman mirrors via `reflector`.

Reboot when the installer finishes and remove the install media.

### Stage 2 — First-boot setup (automatic)

On the first boot the `pi-player-setup` systemd service runs automatically. It executes `ansible-pull` to install Hyprland, pi-player, and all dependencies from this repo, then reboots. No manual steps are required — watch progress with:

```bash
journalctl -u pi-player-setup.service -f
```

After the reboot the web interface is available at `http://<device-ip>:8080/control`.

### Testing the installer in a VM

[`scripts/test-vm.sh`](scripts/test-vm.sh) wraps QEMU with UEFI firmware, a blank virtio disk, the Arch ISO, and SSH forwarding (`localhost:2222 → :22`):

```bash
# Requires: qemu-full, edk2-ovmf
scripts/test-vm.sh            # boot the VM (downloads ISO + creates disk on first run)
scripts/test-vm.sh --fresh    # wipe the disk first, for a clean install test
scripts/test-vm.sh --no-cdrom # boot the installed system without the ISO attached
```

- The target disk inside the VM is `/dev/vda` — enter that at the disk prompt.
- After install, reboot with `--no-cdrom` to boot the installed system.
- SSH into the guest with `ssh -p 2222 <username>@localhost`.
- VM artifacts (ISO, disk image, UEFI NVRAM) live under `.vm/` (gitignored).
- Tunables: `RAM`, `CPUS`, `DISK_SIZE`, `SSH_PORT` (e.g. `DISK_SIZE=40G scripts/test-vm.sh`).

For a verbose install run:

```bash
curl -fsSL .../install.sh | DEBUG=1 bash
```

#### Testing network mounts in the VM

The VM uses QEMU user-mode NAT — it cannot reach your LAN directly. However, `10.0.2.2` is always the host machine's address as seen from inside the VM, so you can expose a Samba share on your host and mount it from the VM without any networking changes.

**1. Create a test share on your host** (one-time setup):

Add to `/etc/samba/smb.conf`:
```ini
[pi-player-test]
    path = /tmp/pi-player-test
    read only = no
    browsable = yes
    guest ok = yes
```

Then:
```bash
mkdir -p /tmp/pi-player-test
sudo systemctl start smb
```

**2. During the VM install**, when `install.sh` asks for the share address, enter:
```
//10.0.2.2/pi-player-test
```

`10.0.2.2` is QEMU's fixed gateway address — it always points to the host regardless of your LAN subnet, so this works on any machine running the VM.

**Stop the test share when done:**
```bash
sudo systemctl stop smb
```

---

## Fleet maintenance

Once devices are provisioned, use the push-mode maintenance playbook to make changes across your fleet from a control machine (laptop or server).

### Setup (once per control machine)

1. **Install Ansible:**
   ```bash
   # Arch
   sudo pacman -S ansible
   # macOS
   brew install ansible
   ```

2. **Create your inventory** from the example:
   ```bash
   cp ansible/inventory.example.yml ansible/inventory.yml
   ```
   Edit `ansible/inventory.yml` to add your device IPs grouped by location. This file is gitignored — it stays on your machine.

3. **Create your secrets file** from the example:
   ```bash
   cp ansible/secrets.example.yml ansible/secrets.yml
   ```
   Edit `ansible/secrets.yml` with the values you want to change. This file is also gitignored.

### Running maintenance

Include only the sections in `secrets.yml` for changes you want to apply — omitted sections are skipped entirely.

```bash
# Apply to all devices
ansible-playbook -i ansible/inventory.yml ansible/maintenance.yml \
  --extra-vars "@ansible/secrets.yml" -K

# Apply to a single location only
ansible-playbook -i ansible/inventory.yml ansible/maintenance.yml \
  --extra-vars "@ansible/secrets.yml" -K --limit lounge
```

`-K` prompts for the sudo password on the target devices.

### Available maintenance tasks

| Variable in `secrets.yml` | Effect |
|---|---|
| `mount_what`, `mount_where`, `mount_user`, `mount_password`, `mount_domain` | Add or update the SMB network mount |
| `device_location` | Update the location label in the pi-player web interface |
| `new_hostname` | Change the device hostname |
| `user_password` | Rotate the user account password |

See [`ansible/secrets.example.yml`](ansible/secrets.example.yml) for the full format and comments.

---

## Documentation

- [`docs/INSTALL.md`](docs/INSTALL.md) — Detailed installation and troubleshooting guide
- [`docs/REMOTE_ADMIN.md`](docs/REMOTE_ADMIN.md) — SSH-based remote administration commands
- [`docs/DEBUG.md`](docs/DEBUG.md) — Local and remote debugging with Neovim/VSCode
