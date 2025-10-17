# Install

## Install Arch Linux on the managed host.
Use provided user_configuration.json file, or set your settings in archinstall.
```bash
archinstall --config user_configuration.json
```

### Set unique settings to this host:
0. OS install drive.
1. root user password.
2. Add user, eg: visuals.
3. Set hostname[]

## Enable SSH
Once installation is complete and the host has rebooted:
Login to device and enable ssh:
```bash
sudo systemctl enable sshd
sudo systemctl start sshd
sudo systemctl status sshd # make sure it has started correctly.
```

## Copy SSH public key from the control node, to the managed host:
```bash
ssh-copy-id user@hostname
```

## Setup SAMBA credentials if auto mounting a share on boot:
If you need to auto mount a SMB share on boot, edit the credentials file:
```
username=visuals
password=examplepass
domain=exampledomain # optional
```

## Run ansible playbook from the control host:
```bash
# Create a copy of the example inventory file:
cp ansible/inventory.example.yml ansible/inventory.yml
# Update the managed host's hostname:

# Run the playbook
ansible-playbook -i ansible/inventory ansible-playbook -K
# Enter managed node BECOME password:
BECOME password: *******
# Wait for the playbook to complete.
```

