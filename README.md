# Pi-Player

[![Go](https://github.com/17xande/pi-player/actions/workflows/build.yml/badge.svg)](https://github.com/17xande/pi-player/actions/workflows/build.yml)

A simple remotely controlled video and image player for a linux based computer. Currently working on Arch with Hyprland.

## Setup
Run setup script:
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

