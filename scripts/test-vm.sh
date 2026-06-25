#!/usr/bin/env bash
#
# test-vm.sh — spin up a disposable UEFI QEMU VM for testing scripts/install.sh
# ----------------------------------------------------------------------------
# Usage:
#   scripts/test-vm.sh            # boot the VM (creates disk/ISO/firmware on first run)
#   scripts/test-vm.sh --fresh    # wipe the disk first, for a clean install test
#   scripts/test-vm.sh --no-cdrom # boot the installed system without the ISO attached
#   scripts/test-vm.sh --testing  # enable SPICE display for clipboard paste support
#                                 # (also pass --testing to install.sh inside the VM)
#
# Tunables (env vars):
#   VM_DIR=.vm   DISK_SIZE=20G   RAM=4096   CPUS=4   SSH_PORT=2222
#   ISO=<path>   (defaults to the latest Arch ISO, downloaded into VM_DIR)
#
# All large/disposable artifacts (ISO, qcow2 disk, NVRAM) live under VM_DIR,
# which is gitignored. The VM uses user-mode networking with host:SSH_PORT ->
# guest:22 forwarded, so once installed you can `ssh -p $SSH_PORT user@localhost`.
#
set -euo pipefail
cd "$(dirname "$0")/.."   # repo root, so VM_DIR is repo-relative regardless of cwd

VM_DIR="${VM_DIR:-.vm}"
DISK_SIZE="${DISK_SIZE:-20G}"
RAM="${RAM:-4096}"
CPUS="${CPUS:-4}"
SSH_PORT="${SSH_PORT:-2222}"
ISO_URL="${ISO_URL:-https://geo.mirror.pkgbuild.com/iso/latest/archlinux-x86_64.iso}"

DISK="$VM_DIR/pi-player.qcow2"
NVRAM="$VM_DIR/OVMF_VARS.4m.fd"
ISO="${ISO:-$VM_DIR/archlinux-x86_64.iso}"

FRESH=0
ATTACH_CDROM=1
TESTING=0
for arg in "$@"; do
  case "$arg" in
    --fresh)    FRESH=1 ;;
    --no-cdrom) ATTACH_CDROM=0 ;;
    --testing)  TESTING=1 ;;
    -h|--help)  sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 1 ;;
  esac
done

command -v qemu-system-x86_64 >/dev/null || { echo "qemu not found: install 'qemu-full'." >&2; exit 1; }

# Locate OVMF firmware across the common packaging layouts.
OVMF_CODE=""
for c in /usr/share/edk2/x64/OVMF_CODE.4m.fd \
         /usr/share/OVMF/OVMF_CODE.4m.fd \
         /usr/share/edk2-ovmf/x64/OVMF_CODE.fd \
         /usr/share/OVMF/OVMF_CODE.fd; do
  [[ -f "$c" ]] && { OVMF_CODE="$c"; break; }
done
[[ -n "$OVMF_CODE" ]] || { echo "OVMF firmware not found: install 'edk2-ovmf'." >&2; exit 1; }
# The writable VARS template lives next to the CODE file.
OVMF_VARS_TEMPLATE="${OVMF_CODE/OVMF_CODE/OVMF_VARS}"

mkdir -p "$VM_DIR"

# ISO: download once.
if [[ ! -f "$ISO" ]]; then
  echo "==> Downloading Arch ISO -> $ISO"
  curl -fL --progress-bar -o "$ISO" "$ISO_URL"
fi

# Disk: create on first run, or recreate with --fresh.
if [[ "$FRESH" == 1 && -f "$DISK" ]]; then
  echo "==> --fresh: removing existing disk"
  rm -f "$DISK"
fi
if [[ ! -f "$DISK" ]]; then
  echo "==> Creating blank $DISK_SIZE disk -> $DISK"
  qemu-img create -f qcow2 "$DISK" "$DISK_SIZE" >/dev/null
fi

# NVRAM: a fresh writable copy whenever the disk is (re)created.
if [[ ! -f "$NVRAM" || "$FRESH" == 1 ]]; then
  echo "==> Initializing UEFI NVRAM from $OVMF_VARS_TEMPLATE"
  cp "$OVMF_VARS_TEMPLATE" "$NVRAM"
fi

# Enable KVM only if available (so this still runs without /dev/kvm).
ACCEL=()
[[ -w /dev/kvm ]] && ACCEL=(-enable-kvm -cpu host) || { echo "==> /dev/kvm unavailable: running without KVM (slow)."; ACCEL=(-cpu max); }

CDROM=()
[[ "$ATTACH_CDROM" == 1 ]] && CDROM=(-cdrom "$ISO" -boot menu=on)

if [[ "$TESTING" == 1 ]]; then
  DISPLAY_ARGS=(-device virtio-serial-pci
                -chardev spicevmc,id=vdagent,name=vdagent
                -device virtserialport,chardev=vdagent,name=com.redhat.spice.0
                -vga qxl
                -display spice-app)
  echo "==> Testing mode: SPICE display with clipboard support"
  echo "==> Run install.sh with --testing inside the VM to enable spice-vdagent"
else
  DISPLAY_ARGS=(-display gtk)
fi

echo "==> Booting VM (disk=$DISK, ram=${RAM}M, cpus=$CPUS, ssh=localhost:$SSH_PORT -> :22)"
exec qemu-system-x86_64 \
  "${ACCEL[@]}" \
  -machine q35 \
  -smp "$CPUS" \
  -m "$RAM" \
  -drive if=pflash,format=raw,readonly=on,file="$OVMF_CODE" \
  -drive if=pflash,format=raw,file="$NVRAM" \
  -drive if=virtio,format=qcow2,file="$DISK" \
  "${CDROM[@]}" \
  -netdev user,id=net0,hostfwd=tcp::"$SSH_PORT"-:22 \
  -device virtio-net,netdev=net0 \
  "${DISPLAY_ARGS[@]}"
