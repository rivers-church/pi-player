# Pi-Player

A simple remotely controlled video and image player for a linux based computer. Currently working on Ubuntu 19.04 and Raspbian.

## Raspberry Pi OS Setup

**OS: [Raspbian Stretch with desktop](https://www.raspberrypi.org/downloads/raspbian/)**\
OR Just regular Ubuntu Desktop on any old machine. v19.04 works.

### Initial Setup

After first boot, follow the instructions to setup the location, screen overscan and network things.\
This should also update the OS.

If the OS update didn't complete successfully, do it manually from the command line:
```bash
sudo apt update && sudo apt upgrade -y
```
Set relevant system settings with `sudo raspi-config`

Wait for network connection on boot to allow the Pi to automatically mount a network location before logging in\
`Boot Options > Wait for Network at Boot > Yes`

`Boot Options > Spash Screen > No`
`Interfacing Options > P2 SSH > Yes`
`Localisation Options > A bunch of stuff here.`

Install additional packages:
```bash
sudo apt install vim git snapd unclutter openssh-server make
```
Reboot to complete snapd installation\
```bash
sudo reboot
```

Install Go language
```bash
sudo snap install go --classic
```

Install Chromium if not already installed. Note that Raspian comes with a different Chromium installation, so you might have to `sudo apt remove chromium-browser`
```bash
sudo snap install chromium
```

Get the pi-player project:\
```bash
mkdir -p ~/Software
cd ~/Software
git clone https://github.com/17xande/pi-player
```

Build Project by running `make`.\
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

## GUI Changes
- Set Background to black.
- Set Dock to small.
- Set Dock to autohide.
- Hide Desktop icons.