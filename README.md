# Pi-Player

A simple remotely controlled video and image player for a linux based computer. Currently working on Ubuntu 22.04. Not currently working in Raspbian.

Install additional packages:
```bash
sudo apt install neovim git unclutter ssh
```

Enable ssh:
```bash
sudo systemctl start ssh
sudo systemctl enable ssh
```

If you'll be changing code on the device, install Go language:
```bash
sudo snap install go --classic
```

Install Chromium:
```bash
sudo snap install chromium
```

Get the pi-player project:
```bash
mkdir -p ~/Software
cd ~/Software
git clone https://github.com/17xande/pi-player
```

Change directory to `pi-player` and build the module:
```bash
cd pi-player
go build
```

Setup the app to start on boot:
```bash
mkdir -p ~/.config/systemd/user
cp pi-player.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable pi-player
```

Setup `unclutter` to start on boot:
```bash
cp unclutter.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable unclutter
```

Add the current user to the video group so that they can play videos,
and to the input group so that they can read the USB remote events:
```bash
usermod -a -G video,input $(whoami)
```

Test project:
```bash
systemctl --user start pi-player
```

Check the status of the running service:
```bash
systemctl --user status pi-player
```

Access the server from a browset to make sure it's running properly. Use the following address:\
`<device-ip-address>:8080/control`


Reboot the PC and make sure that the program still runs on boot correctly.

TODO:
GIO USB mount support

Restart the Pi again and make sure everything boots up and works as expected. A black screen should be displayed once the Pi has booted and you should have control from the webpage `<ip-address>:8080/control`

## System Preferences Changes
- Set bluetooth off.
- Set Background to black or other picture.
- Set Appearance to Dark.
- Set Dock to autohide.
- Set notifications to Do Not Disturb.
- Set Lock screen notifications to disabled.
- Privacy settings:
    - Set Connectivity checking to disabled.
    - Set Screen/Blank screen delay to never.
    - Set Screen/Automatic screen lock to disabled.
    - Set Screen/Lock screen on suspend to disabled.
    - Set Screen/Show notifications on lock screen to disabled.
- Set Sharing/remote desktop to On:
    - Set Remote control to enabled.
- Set Sound/System volume to 100%.
- Set Sound/Volume levels/System sounds to 0%.
- Set Power/Power mode to Performance.
- Set Power/Screen blank to Never.
- Set Power/Automatic Suspend to Off.
- Set Displays/refresh rate to 50Hz.
- Set Date & Time/Automatic Timezone to enabled.

### Open Software & updates and make the following changes under Updates:
- Subscribed to: Security updates only.
- Automatically check for updates: Every two weeks.
- When there are security updates: Download and install automatically.
- When there are other updates: Display every two weeks.
- Notify me of a new Ubuntu version: Never.
