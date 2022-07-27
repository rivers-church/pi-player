# Pi-Player

A simple remotely controlled video and image player for a linux based computer. Currently working on Ubuntu 22.04. Not currently working in Raspbian.

## Ubuntu Linux Setup:
Install additional packages:
```bash
sudo apt install neovim git unclutter ssh
sudo snap install chromium
```

Enable ssh:
```bash
sudo systemctl start ssh
sudo systemctl enable ssh
```

Add the crrentuser to the input group so that they can read the USB remote events:
```bash
sudo usermod -a -G input $(whoami)
```
If you'll be changing code on the device, get the tools and source code:
```bash
sudo snap install go --classic
mkdir -p ~/Software
cd ~/Software
git clone https://github.com/17xande/pi-player
# build the project.
cd pi-player
go build
```

Setup the app and `unclutter` to start on boot:
NOTE: update the home path in the `pi-player.service` file to reflect your home folder.
```bash
mkdir -p ~/.config/systemd/user
cp services/*.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable pi-player
systemctl --user enable unclutter
```

Test project:
```bash
systemctl --user start pi-player
# Check the status of the running service:
systemctl --user status pi-player
```

Access the server from a browset to make sure it's running properly. Use the following address:\
`<device-ip-address>:8080/control`


Reboot the PC and make sure that the program still runs on boot correctly.

Restart the Pi again and make sure everything boots up and works as expected. A black screen should be displayed once the Pi has booted and you should have control from the webpage `<ip-address>:8080/control`

## System Preferences Changes
```bash
# Set Background to black or other picture.
gsettings set org.gnome.desktop.background picture-uri ""
gsettings set org.gnome.desktop.background picture-uri-dark ""
gsettings set org.gnome.desktop.background primary-color '#000000'
# Set Appearance to Dark.
gsettings set org.gnome.desktop.interface color-scheme 'prefer-dark'
# Set Dock to autohide.
gsettings set org.gnome.shell.extensions.dash-to-dock dock-fixed false
# Set notifications to Do Not Disturb.
gsettings set org.gnome.desktop.notifications show-banners false
# Set Lock screen notifications to disabled.
gsettings set org.gnome.desktop.notifications show-in-lock-screen false
# Stop showing update notifications.
gsettings set com.ubuntu.update-notifier no-show-notifications true
# Privacy settings:

# Screen settings:
# Set Blank screen delay to never.
gsettings set org.gnome.desktop.screensaver lock-delay 0
# Set Automatic screen lock to disabled.
gsettings set org.gnome.desktop.screensaver lock-enabled false
# Set Lock screen on suspend to disabled.
gsettings set org.gnome.desktop.screensaver ubuntu-lock-on-suspend false
# Set Show notifications on lock screen to disabled.
gsettings set org.gnome.desktop.notifications show-in-lock-screen false
# Set Sharing/remote desktop to On.
# Set Remote control to enabled.
gsettings set org.gnome.desktop.remote-desktop.rdp enable true
gsettings set org.gnome.desktop.remote-desktop.vnc view-only false
# Set power settings.
# Set Screen blank to Never.
gsettings set org.gnome.desktop.session idle-delay 0
# Set Automatic Suspend to Off.
gsettings set org.gnome.settings-daemon.plugins.power sleep-inactive-ac-type 'nothing'
```
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

### Open Software & updates and make the following changes under Updates:
- Subscribed to: Security updates only.
- Automatically check for updates: Every two weeks.
- When there are security updates: Download and install automatically.
- When there are other updates: Display every two weeks.
- Notify me of a new Ubuntu version: Never.

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

### Unlock the keyring:
We need to do this because we are automatically logging in.
- Open passwords and Keys.
- Right click on Passwords/Login.
- Click on Change password.
- Enter your current password.
- When asked for a new password, leave it empty.
- Click Ok to confirm.
