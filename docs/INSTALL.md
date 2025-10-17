# Pi-Player Installation Guide

This guide covers the complete installation process for setting up a pi-player kiosk system on Arch Linux with Hyprland.

## Prerequisites

On your **control machine** (laptop/desktop):
- Ansible installed (`sudo pacman -S ansible` or equivalent)
- SSH key pair generated (`ssh-keygen` if you don't have one)
- Ansible AUR collection: `ansible-galaxy collection install kewlfft.aur`

On the **target machine**:
- Physical access for initial setup
- Wired Ethernet connection (recommended for installation)
- USB drive with Arch Linux installer

---

## Step 1: Install Arch Linux

Boot from the Arch Linux installer USB on the target machine.

### Option A: Using archinstall with saved configuration

```bash
# Load the installation configuration
archinstall --config user_configuration.json
```

### Option B: Interactive archinstall

```bash
archinstall
```

Configure these settings during installation:
- **Disk configuration**: Select the target drive
- **Disk encryption**: None
- **Bootloader**: systemd-boot
- **Swap**: True (recommended)
- **Hostname**: Set a unique name (e.g., `pi-player-01`, `kiosk-lobby`)
- **Root password**: Set a secure password
- **User account**: Create a user (e.g., `visuals`, `kiosk`)
  - Set user password
  - Add to sudo group
- **Profile**: Select "Minimal"
- **Audio**: pipewire
- **Network configuration**: systemd-networkd
- **Additional packages**: `openssh` and `python` (required for ansible)

Complete the installation and reboot into the new system.

---

## Step 2: Enable SSH Access

After the first boot, log in to the target machine and enable SSH:

```bash
# Enable and start SSH daemon
sudo systemctl enable sshd
sudo systemctl start sshd

# Verify SSH is running
sudo systemctl status sshd

# Check the IP address
ip addr show

# Or hostname
hostname
```

Make note of the IP address or hostname (e.g., `192.168.1.100`).

---

## Step 3: Configure SSH Key Authentication

From your **control machine**, copy your SSH public key to the target:

```bash
# Replace 'user' and 'hostname' with actual values
ssh-copy-id user@192.168.1.100

# Test SSH connection
ssh user@192.168.1.100
```

If successful, you should be able to SSH without entering a password.

---

## Step 4: Configure Network Share (Optional)

If you need to auto-mount an SMB network share on boot:

### Create mount configuration

1. Copy the example mount file:
   ```bash
   cp services/mnt-networkshare.mount.example services/mnt-networkshare.mount
   ```

2. Edit `services/mnt-networkshare.mount`:
   ```ini
   [Unit]
   Description=Network Share for Media Files

   [Mount]
   What=//server.example.com/media
   Where=/mnt/networkshare
   Type=cifs
   Options=credentials=/etc/samba/credentials,uid=1000,gid=1000,iocharset=utf8

   [Install]
   WantedBy=multi-user.target
   ```

   Update the `What=` line with your SMB server and share path.

### Create credentials file

1. Copy the example credentials file:
   ```bash
   cp services/samba-credentials.example services/samba-credentials
   ```

2. Edit `services/samba-credentials`:
   ```ini
   username=your_smb_username
   password=your_smb_password
   domain=WORKGROUP
   ```

---

## Step 5: Configure Ansible Inventory

1. Create your inventory file:
   ```bash
   cp ansible/inventory.example.yml ansible/inventory.yml
   ```

2. Edit `ansible/inventory.yml`:
   ```yaml
   devices:
     hosts:
       pi-player-01:
         ansible_host: 192.168.1.100
         ansible_user: visuals
   ```

   Update:
   - `pi-player-01`: A descriptive name for this device
   - `ansible_host`: The IP address or hostname
   - `ansible_user`: The username created during installation

---

## Step 6: Run Ansible Playbook

From your **control machine**, run the playbook:

```bash
# Navigate to the project directory
cd pi-player

# Run the playbook
ansible-playbook -i ansible/inventory.yml ansible/playbook.yml -K

# You will be prompted for the BECOME password (sudo password on target)
BECOME password: ********
```

The playbook will:
- Install essential packages (Hyprland, Chromium, swayosd, etc.)
- Configure automatic login
- Set up networking with DNS domain support
- Configure Hyprland for kiosk mode
- Install and configure pi-player service
- Mount network share (if configured)
- Install AUR packages (yay, neovim, ghostty)

**Note**: The playbook may take 15-30 minutes depending on internet speed.

---

## Step 7: Verify Installation

After the playbook completes successfully:

1. **Reboot the target machine**:
   ```bash
   ssh user@192.168.1.100 'sudo reboot'
   ```

2. **Check services are running** (after reboot):
   ```bash
   ssh user@192.168.1.100

   # Check Hyprland is running
   ps aux | grep Hyprland

   # Check pi-player service
   systemctl --user status pi-player

   # Check network mount (if configured)
   systemctl status mnt-networkshare.mount
   ls /mnt/networkshare
   ```

3. **Access web interface**:

   Open a browser on your control machine:
   - Control Panel: `http://192.168.1.100:8080/control`
   - Media Viewer: `http://192.168.1.100:8080/viewer`
   - Settings: `http://192.168.1.100:8080/settings`

---

## Troubleshooting

### Ansible Playbook Fails

**AUR package installation fails with 502 error:**
- This is usually a temporary AUR outage
- Wait a few minutes and retry the playbook
- The playbook is idempotent, so it's safe to re-run

**Cannot connect to target machine:**
- Verify SSH is running: `ssh user@hostname`
- Check firewall settings on target
- Verify ansible_host in inventory.yml is correct

### Network Share Not Mounting

**Check mount status:**
```bash
systemctl status mnt-networkshare.mount
journalctl -u mnt-networkshare.mount
```

**Common issues:**
- Incorrect credentials in `/etc/samba/credentials`
- Network path in mount file is wrong
- SMB server is unreachable
- Firewall blocking SMB ports (445, 139)

**Manual mount test:**
```bash
sudo mount -t cifs //server/share /mnt/networkshare -o credentials=/etc/samba/credentials,uid=1000,gid=1000
```

### Pi-Player Not Starting

**Check service status:**
```bash
systemctl --user status pi-player
journalctl --user -u pi-player -n 50
```

**Common issues:**
- Chromium not installed
- Port 8080 already in use
- Hyprland not running

### DNS Resolution Issues

See [REMOTE_ADMIN.md](REMOTE_ADMIN.md) for DNS troubleshooting and `resolvectl` commands.

---

## Next Steps

- Configure media directories in pi-player settings: `http://kiosk-ip:8080/settings`
- Set up remote debugging if needed: [DEBUG.md](DEBUG.md)
- Review remote administration commands: [REMOTE_ADMIN.md](REMOTE_ADMIN.md)
- Add the kiosk to your monitoring/management system

---

## Saving archinstall Configuration

To save your archinstall configuration for future installations:

1. **During installation**, when prompted to save configuration:
   - Select "Yes" to save
   - Choose a location (use a second USB drive if available)

2. **After installation**, copy the config from `/var/log/archinstall/`:
   ```bash
   # Copy from the installed system
   sudo cp /var/log/archinstall/user_configuration.json /root/

   # Or via SSH after first boot
   scp user@hostname:/var/log/archinstall/user_configuration.json ./
   ```

3. **Important**: Remove sensitive data before committing:
   - User passwords
   - Disk encryption passwords
   - SSH keys

---

## Additional Resources

- [Hyprland Documentation](https://wiki.hyprland.org/)
- [Arch Linux Installation Guide](https://wiki.archlinux.org/title/Installation_guide)
- [Ansible Documentation](https://docs.ansible.com/)

