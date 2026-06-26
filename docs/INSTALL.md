# Pi-Player Installation Guide

For a step-by-step provisioning walkthrough see the [README](../README.md). This document covers prerequisites, troubleshooting, and the maintenance playbook in more detail.

---

## Prerequisites

**On your control machine** (for fleet maintenance after provisioning):
- Ansible (`sudo pacman -S ansible` or `brew install ansible`)
- SSH access to the target devices (keys copied via `ssh-copy-id`)

**On the target machine:**
- Physical access for initial setup
- Wired Ethernet (recommended — WiFi requires `iwctl` from the live ISO)
- USB drive with the [Arch Linux ISO](https://archlinux.org/download/)

---

## Stage 1: Base install

Boot from the Arch ISO in UEFI mode and run:

```bash
curl -fsSL https://raw.githubusercontent.com/rivers-church/pi-player/master/scripts/install.sh | bash
```

The installer will prompt for hostname, username/password, root password, network config (DHCP or static — static fields pre-fill from DHCP), an optional SMB network mount, and the target disk. Everything else (timezone, locale, keymap, mirrors) is auto-detected from your IP.

Reboot and remove the install media when done.

---

## Stage 2: First-boot Ansible setup (automatic)

The `pi-player-setup` service runs `ansible-pull` on first boot. It installs Hyprland, pi-player, and all dependencies, then reboots. No manual action is needed.

Watch progress over SSH:
```bash
ssh <user>@<device-ip> 'journalctl -u pi-player-setup.service -f'
```

After the final reboot, verify the web interface is up:
```
http://<device-ip>:8080/control
```

---

## Fleet maintenance (push playbook)

For changes after provisioning — adding mounts, rotating passwords, updating location labels — use the push-mode maintenance playbook from your control machine.

### Inventory setup

```bash
cp ansible/inventory.example.yml ansible/inventory.yml
```

Edit `ansible/inventory.yml` to list your device IPs grouped by location. This file is gitignored.

```yaml
all:
  vars:
    ansible_user: pi

  children:
    lounge:
      hosts:
        192.168.1.100:

    hall:
      hosts:
        192.168.1.200:
```

### Secrets file

```bash
cp ansible/secrets.example.yml ansible/secrets.yml
```

Edit `ansible/secrets.yml` and include only the sections for changes you want to apply. Omitted sections are skipped. This file is gitignored — it stays on your machine.

### Running the playbook

```bash
# All devices
ansible-playbook -i ansible/inventory.yml ansible/maintenance.yml \
  --extra-vars "@ansible/secrets.yml" -K

# Single location
ansible-playbook -i ansible/inventory.yml ansible/maintenance.yml \
  --extra-vars "@ansible/secrets.yml" -K --limit lounge
```

---

## Troubleshooting

### First-boot setup fails

```bash
# Check the setup service logs
ssh <user>@<device-ip> 'journalctl -u pi-player-setup.service -n 100'
```

The service is a one-shot that runs `ansible-pull`. Common causes: no internet on first boot, AUR outage (retry), DNS not yet ready. The service is safe to re-run:

```bash
ssh <user>@<device-ip> 'sudo systemctl start pi-player-setup.service'
```

### Network share not mounting

```bash
# Check mount unit status
ssh <user>@<device-ip> 'systemctl status <unit>.mount'
ssh <user>@<device-ip> 'journalctl -u <unit>.mount -n 50'

# Manual mount test (diagnoses credential or path issues)
ssh <user>@<device-ip> 'sudo mount -t cifs //server/share /mnt/test \
  -o credentials=/etc/samba/credentials,uid=1000,gid=1000'
```

Common causes: wrong credentials in `/etc/samba/credentials`, SMB server unreachable, firewall blocking ports 445/139.

### Pi-player not starting

```bash
ssh <user>@<device-ip> 'systemctl --user status pi-player'
ssh <user>@<device-ip> 'journalctl --user -u pi-player -n 50'
```

### Ansible maintenance playbook cannot connect

```bash
# Verify SSH works
ssh <user>@<device-ip>

# Verify sudo works
ssh <user>@<device-ip> 'sudo whoami'

# Check ansible_user matches the username set during install
grep ansible_user ansible/inventory.yml
```

---

## Additional resources

- [Hyprland documentation](https://wiki.hyprland.org/)
- [Arch Linux installation guide](https://wiki.archlinux.org/title/Installation_guide)
- [Ansible documentation](https://docs.ansible.com/)
