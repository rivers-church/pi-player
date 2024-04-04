# Pi-Player

![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/17xande/pi-player/go.yml)

A simple remotely controlled video and image player for a linux based computer. Currently working on Ubuntu 22.04. Not currently working in Raspbian.

## Setup
Run setup script:
```bash
sudo ./setup.sh
```

Logout or restart.

Test project:
```bash
systemctl --user start pi-player
# Check the status of the running service:
systemctl --user status pi-player
```

Access the server from a browset to make sure it's running properly. Use the following address:.`http://<device-ip-address>:8080/control`

Reboot the PC and make sure that the program still runs on boot correctly.

A black screen should be displayed once the Pi has booted and you should have control from the webpage `<ip-address>:8080/control`

## Preferences that can't be set with gsettings:
- Set bluetooth off.
- Set Privacy/Connectivity checking to disabled.
- Sound Settings:
  - Set System volume to 100%.
  - Set Volume levels/System sounds to 0%.
- Power Settings:
  - Set Power mode to Performance (Some machines don't have this).
- Set Displays/refresh rate to 50Hz.
- Set Date & Time/Automatic Timezone to enabled.

### Setup Samba shares if required:
```bash
sudo apt install samba
# setup user account. Note: this user has to already exist locally.
sudo smbpasswd -a sandtonvisuals
# enter password for this user. It can be the same password as the local user.

# creat directory that will be shared.
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

