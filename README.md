# Pi-Player

A simple remotely controlled video and image player for a linux based computer. Currently working on Ubuntu 19.04 and Raspbian.

## Raspberry Pi OS Setup

**OS: [Raspbian Stretch with desktop](https://www.raspberrypi.org/downloads/raspbian/)**

### Initial Setup

After first boot, follow the instructions to setup the location, screen overscan and network things.\
This should also update the OS.

If the OS update didn't complete successfully, do it manually from the command line: `sudo apt update && sudo apt upgrade -y`\
Set relevant system settings with `sudo raspi-config`

Wait for network connection on boot to allow the Pi to automatically mount a network location before logging in\
`Boot Options > Wait for Network at Boot > Yes`

`Boot Options > Spash Screen > No`
`Interfacing Options > P2 SSH > Yes`
`Localisation Options > A bunch of stuff here.`

Install additional packages:\
`sudo apt install vim git snapd`
Reboot to complete snapd installation
`sudo reboot`

Install Go language
`sudo snap install go --classic`

Install Chromium if not already installed. Note that Raspian comes with a different Chromium installation, so you might have to `sudo apt remove chromium-browser`
`sudo snap install chromium`

Get the pi-player project:
`mkdir -p ~/Software`
`cd ~/Software`
`git clone https://github.com/17xande/pi-player`\

Build Project by running `make`.\
Setup the app to start on boot:\
```bash
mkdir -p ~/.config/systemd/user
cp pi-player.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable pi-player
```

Add the crrentuser to the video group so that they can play videos,
and to the input group so that they can read the USB remote events:\
```bash
usermod -a -G video,input $(whoami)
```

Test project:\
```bash
sudo systemctl start pi-player
```

Check the status of the running service:\
`sudo systemctl status pi-player`

Access the server from a browset to make sure it's running properly. Use the following address:\
`<device-ip-address>:8080/control`


Reboot the PC and make sure that the program still runs on boot correctly.

TODO:
GIO USB mount support
unclutter setup

Restart the Pi again and make sure everything boots up and works as expected. A black screen should be displayed once the Pi has booted and you should have control from the webpage `<ip-address>:8080/control`
