# Pi-Player

[![Go](https://github.com/rivers-church/pi-player/actions/workflows/build.yml/badge.svg)](https://github.com/rivers-church/pi-player/actions/workflows/build.yml)

A simple remotely controlled video and image player for a linux based computer. Currently working on Arch with Hyprland.

## Provisioning a new unit

Provisioning a kiosk is a two-stage process: a base Arch install from the live
ISO, followed by the Ansible playbook that configures the pi-player environment.

### 1. Base Arch install (from the live ISO)

Boot the target machine from an Arch Linux ISO **in UEFI mode**, connect it to
the network (ethernet is automatic; for wifi use `iwctl`), then run:

```bash
curl -fsSL https://raw.githubusercontent.com/rivers-church/pi-player/master/scripts/install.sh | bash
```

The installer ([`scripts/install.sh`](scripts/install.sh)) replicates the old
archinstall config and prompts only for the things that genuinely vary per unit:

- **Prompts:** hostname, username + password, root password, target disk
  (with a typed confirmation before wiping), and DHCP vs. static IP.
- **Auto-detected from your IP** (no prompts): timezone, the closest/fastest
  pacman mirrors (via `reflector`), system locale, and console keymap.
- **Layout:** systemd-boot + UKI, GPT with a 1 GiB FAT32 ESP, and btrfs
  subvolumes (`@`, `@home`, `@log`, `@pkg`) with `compress=zstd`.
- **Installed/enabled:** systemd-networkd + resolved, NTP, zram swap (zstd),
  pipewire, ufw, `python` (required by Ansible), and `openssh` (enabled) so the
  playbook below can connect immediately after first boot.

Reboot when it finishes and remove the install media.

> Passwords are entered interactively during install, so there is **no**
> committed credentials file. (The previous `scripts/user_credentials.json` has
> been removed.) `scripts/user_configuration.json` is kept only as a reference
> of the original archinstall layout.

### Testing the installer in a VM

Before running `install.sh` on real hardware, test it in a disposable UEFI VM.
[`scripts/test-vm.sh`](scripts/test-vm.sh) wraps QEMU with all the right flags
(UEFI firmware, a blank virtio disk, the Arch ISO, and `localhost:2222 -> :22`
SSH forwarding):

```bash
# Requires: qemu-full, edk2-ovmf
scripts/test-vm.sh            # boot the VM (downloads ISO + creates disk on first run)
scripts/test-vm.sh --fresh    # wipe the disk first, for a clean install test
scripts/test-vm.sh --no-cdrom # boot the installed system without the ISO attached
```

Notes:

- The target disk inside the VM is `/dev/vda` (virtio), so enter `/dev/vda` at
  the installer's disk prompt.
- After a successful install, reboot with `--no-cdrom` so it boots the installed
  system instead of the live ISO again.
- Once SSH is up you can reach the guest with `ssh -p 2222 <username>@localhost`.
- All VM artifacts (ISO, disk image, UEFI NVRAM) live under `.vm/`, which is
  gitignored. Tunables: `RAM`, `CPUS`, `DISK_SIZE`, `SSH_PORT` (e.g.
  `DISK_SIZE=40G RAM=8192 scripts/test-vm.sh`).

To debug a failing install run, the installer accepts a debug flag:

```bash
curl -fsSL https://raw.githubusercontent.com/rivers-church/pi-player/master/scripts/install.sh | DEBUG=1 bash
```

### 2. Configure with Ansible

From your workstation, point the inventory at the new host (reachable over SSH
as `<username>@<hostname>`) and run the playbook:

```bash
ansible-playbook -i ansible/inventory.yml ansible/playbook.yaml
```

Logout or restart.

Test project:
```bash
systemctl --user start pi-player
# Check the status of the running service:
systemctl --user status pi-player
# You might have to update the config file at ~/.config/pi-player/config.json
```

Access the server from a browser to make sure it's running properly. Use the following address:.`http://<device-ip-address>:8080/control`

## Documentation

- [DEBUG.md](DEBUG.md) - Debugging guide for local and remote debugging with Neovim/VSCode
- [REMOTE_ADMIN.md](REMOTE_ADMIN.md) - Remote administration commands for managing kiosks over SSH
- [CLAUDE.md](CLAUDE.md) - Instructions for Claude Code AI assistant

### Setup Samba shares if required:
```bash
sudo apt install samba
# setup user account. Note: this user has to already exist locally.
sudo smbpasswd -a sandtonvisuals
# enter password for this user. It can be the same password as the local user.

# create directory that will be shared.
mkdir -p ~/Documents/media
# edit the samba configuration file.
sudo vim /etc/samba/smb.conf
```

At the bottom of the file, add the following:
```samba
[media]
    comment = Twinkle 2 media
    path = /home/sandtonvisuals/documents/media
    read only = no
    browsable = yes
```
```bash
# restart the samba service
sudo systemctl restart smbd
```

