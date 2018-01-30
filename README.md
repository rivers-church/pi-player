# Pi-Player

A simple remotely controlled video and image player for Raspberry Pi.

### TODO
The setup guide below is overly complicated and probably has many faults. It will be simplified in the future.

## Raspberry Pi OS Setup
**OS: [Raspbian Stretch Lite](https://www.raspberrypi.org/downloads/raspbian/)**

### Initial Setup:
Update OS: `sudo apt update && sudo apt upgrade -y`  
Set relevant system settings with `sudo raspi-config`

Wait for network connection on boot to allow the Pi to automatically mount a network location before logging in  
`Boot Options > Wait for Network at Boot > Yes`

`Boot Options > Spash Screen > No`  
`Localisation Options > A bunch of stuff here.`

Install additional packages:
`sudo apt install git vim chromium-browser libgtk-3-0 xorg ttf-freefont`

### Setup network location mount on boot:
Create a directory for the mount:
`sudo mkdir /media/visuals`  
Add the following entry to the `/etc/fstab` file (use tabs to separate each section):  
`//host/path /media/visuals  cifs    username=user,password=pass,iocharset=utf8,sec=ntlm   0       0`
**Note:** replace spaces with */040*
Reboot the Pi: `sudo reboot`  
Check that your folder is mounted at boot: `ls /media/visuals`

Find the `systemd` service for this mount:  
`systemctl | grep /media/visuals`  
Something similar to the following should be returned:  
`media-visuals.mount`  
Take note of that mount service for later.

[Install GO](https://golang.org/doc/install)  
Get the pi-player project: `go get github.com/17xande/pi-player`
Update the *location* and *directory* settings in the `config.json` file. Leave the remote options as they are:

```json
{
  "location": "Foyer",
  "directory": "/media/visuals/foyer"
}
```

Build Project  
Run project to make sure it runs  
Setup the app to start on boot:  
Create a file called `pi-player.service` with the following contents:  
```
[Unit]
Description=Pi Player
ConditionPathExists=/home/pi/go/src/github.com/17xande/pi-player
ConditionPathExists=/media/visuals/Kidszone
# network must be ready AND the visuals folder must be mounted.
# add the mount service to this line
After=network.target media-visuals.mount

[Service]
Type=simple
User=piplayerservice
Group=piplayerservice
LimitNOFILE=1024

Restart=on-failure
RestartSec=10
# There’s a bug in the current implementation of systemd in the raspberry 
# pi, so for now use StartLimitInterval instead of StartLimitIntervalSec
#StartLimitIntervalSec=60
StartLimitInterval=60

WorkingDirectory=/home/pi/go/src/github.com/17xande/pi-player
ExecStart=/home/pi/go/src/github.com/17xande/pi-player/pi-player

# Make sure log directory exists
PermissionsStartOnly=true
ExecStartPre=/bin/mkdir -p /var/log/pi-player
# syslog doesn’t exist by default in the Pi, so this line can be left out
#ExecStartPre=/bin/chown syslog:adm /var/log/pi-player
#ExecStartPre=/bin/chmod 755 /var/log/pi-player
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=pi-player

[Install]
WantedBy=multi-user.target
```

Create a user that will run the program:
```bash
cd /tmp
sudo useradd piplayerservice -s /sbin/nologin -M
```

Add the user to the video group so that they can play videos, and to the input group so that they can read the USB remote events:
`usermod -a -G video,input piplayerservice`

Move the `pi-player.service` file to the correct location and make it executable:
```bash
sudo mv pi-player.service /lib/systemd/system/.
sudo chmod 755 /lib/systemd/system/pi-player.service
```

Enable and start the service:
```bash
sudo systemctl enable pi-player
sudo systemctl start pi-player
```

Check the status of the running service:  
`sudo systemctl status pi-player`

Access the server from a browset to make sure it's running properly. Use the following address:  
`<device-ip-address>:8080/control`

Setup Syslog for the app to log to the right place:  
Edit `/etc/rsyslog.conf` and uncomment the following lines:  
```
# provides UDP syslog reception
#module(load="imudp")
#input(type="imudp" port="514")

# provides TCP syslog reception
module(load="imtcp") #UNCOMMENT
input(type="imtcp" port="514") #UNCOMMENT

# syntax for forcing listener address
# input(type="imtcp" port="514" address="127.0.0.1")

```

Restart the service, and check if the TCP listener on port 514 is visible:  
```bash
$ sudo systemctl restart rsyslog
# show syslog logs using systemd journal
$ sudo journalctl -u rsyslog
# check 
$ netstat -an | grep "LISTEN "
```
You should see port 514 being used, along with a few other ports.

Tail the log to see that it’s working correctly:  
`sudo journalctl -f -u piplayerservice`

Configure syslog to have the log sent to the right folder:  
Create the file `/etc/rsyslog.d/30-pi-player.conf` and add the following contents:
```
if $programname == 'pi-player' or $syslogtag == 'pi-player' then /var/log/pi-player/pi-player.log
& stop
```

Restart the syslog service:  
`sudo systemctl restart syslog`

Restart the pi-player service and check that the logs are going to the new log location of `/var/log/pi-player/pi-player.log`
```bash
sudo systemctl restart pi-player
tail -f /var/log/pi-player/pi-player.log
```

Things should be logged as the application runs.

Reboot the Pi and make sure that the program still runs on boot correctly.

Setup X server to start on boot:
Create a new systemd service file at `/lib/systemd/system/x.service`
```
[Unit]
Description=X server
After=pi-player.service

[Service]
Type=simple
Restart=on-failure
RestartSec=10
StartLimitInterval=60

ExecStart=/usr/bin/X -nocursor
# run unclutter with no delay to remove the cursor from the screen
# for some reason this isn’t working so I’ve commented out for now.
#ExecStartPre=/usr/bin/unclutter -idle 0

[Install]
WantedBy=multi-user.target
```

Enable and start the service:
```bash
sudo systemctl enable x
sudo systemctl start x
```

The screen should go black as the X server starts

Restart the Pi again and make sure everything boots up and works as expected. A black screen should be displayed once the Pi has booted and you should have control from the webpage `<ip-address>:8080/control`

