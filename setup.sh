#! /bin/bash

# Install required packages.
sudo apt install neovim git unclutter ssh -y
sudo snap install chromium

# Start ssh and set it to autostart.
sudo systemctl start ssh
sudo systemctl enable ssh

# Remove update popup notification:
sudo apt remove update-notifier -y

# Setup pi-player and unclutter to run at boot.
mkdir -p ~./config/systemd/user
cp services/*.services ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable pi-player
systemctl --user enable unclutter

# Download the pi-player binary.
mkdir -p ~/.local/bin
wget -O ~/.local/bin/pi-player https://github.com/17xande/pi-player/releases/latest/download/pi-player

# Add the current user to the input group so that they can read the USB remote events.
# Note: a logout or restart is required for this to take effect.
sudo usermod -a -G input $USER

echo "Logout or restart your computer for changes to take effect."

# System preferences changes, to make this work like a kiosk.

# Remove desktop background for both light and dark appearance mode.
gsettings set org.gnome.desktop.background picture-uri ""
gsettings set org.gnome.desktop.background picture-uri-dark ""
# Set desktop color to black.
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

