#!/usr/bin/env bash
#
# pi-player Arch Linux installer
# ------------------------------
# Run from a booted Arch Linux ISO (UEFI mode):
#
#   curl -fsSL https://raw.githubusercontent.com/rivers-church/pi-player/master/scripts/install.sh | bash
#
# Reproduces the archinstall config in scripts/user_configuration.json:
#   - systemd-boot + UKI, btrfs (@ @home @log @pkg, compress=zstd), 1GiB FAT32 ESP
#   - systemd-networkd (DHCP or static), systemd-resolved, NTP via timesyncd
#   - zram swap (zstd), pipewire audio, ufw firewall, openssh (for Ansible)
#   - locale from geolocation (e.g. en_ZA.UTF-8), us keymap, auto-detected timezone (falls back to UTC)
#
# Interactive prompts: hostname, username/password, root password, target disk,
# DHCP vs static IP, timezone (auto-detected), mirror country.
#
set -euo pipefail

# ---------------------------------------------------------------------------
# Output helpers — palette: orange (primary) + white/grey + blue complement
# Detects 256-colour support; falls back to 16-colour for the Arch live TTY
# (the Linux console renders \e[38;5;208m as red — \e[33m is brown-orange).
# ---------------------------------------------------------------------------
c_reset=$'\e[0m'
c_bold=$'\e[1m'
c_dim=$'\e[2m'
_ncolors=$(tput colors 2>/dev/null || echo 0)
if [[ "$_ncolors" -ge 256 ]]; then
  c_orange=$'\e[38;5;208m' # primary
  c_amber=$'\e[38;5;214m'  # warnings (analogous to orange)
  c_blue=$'\e[38;5;39m'    # complement to orange (info accents)
  c_green=$'\e[38;5;42m'   # success
  c_red=$'\e[38;5;203m'    # errors
  c_grey=$'\e[38;5;240m'   # dark grey (frame)
  c_lgrey=$'\e[38;5;250m'  # light grey (subtitle)
else
  # 16-colour fallback: \e[33m = CGA "yellow" = brown-orange on Linux TTY
  c_orange=$'\e[33m'
  c_amber=$'\e[1;33m' # bright yellow (analogous)
  c_blue=$'\e[36m'    # cyan (complement)
  c_green=$'\e[32m'
  c_red=$'\e[31m'
  c_grey=$'\e[2;37m' # dim white = dark grey
  c_lgrey=$'\e[37m'  # light grey
fi
info() { printf '%s==>%s %s\n' "$c_orange" "$c_reset" "$*"; }
ok() { printf '%s==>%s %s\n' "$c_green" "$c_reset" "$*"; }
warn() { printf '%s==>%s %s\n' "$c_amber" "$c_reset" "$*" >&2; }
die() {
  _DYING=1
  printf '%s ✗ %s%s\n' "$c_red" "$*" "$c_reset" >&2
  exit 1
}

# ---------------------------------------------------------------------------
# Debug mode
#   Enable with either:
#     curl ... | DEBUG=1 bash
#     curl ... | bash -s -- --debug
#   dbg() prints extra diagnostics; PP_TRACE=1 additionally enables `set -x`
#   (noisy, and may echo values — avoid while entering passwords).
# ---------------------------------------------------------------------------
DEBUG="${DEBUG:-0}"
TESTING="${TESTING:-0}"
for _arg in "$@"; do
  case "$_arg" in
  -d | --debug | -v | --verbose) DEBUG=1 ;;
  --trace)
    DEBUG=1
    PP_TRACE=1
    ;;
  --testing) TESTING=1 ;;
  esac
done
dbg() { [[ "$DEBUG" == 1 ]] && printf '%s  [dbg]%s %s\n' "$c_dim" "$c_reset" "$*" >&2 || true; }
_DYING=0
_on_error() {
  [[ "$_DYING" == 1 ]] && return
  local line=$1 cmd="$2" code=$3
  if [[ "$DEBUG" == 1 ]]; then
    printf '%s ✗ Error at line %d (exit %d): %s%s\n' "$c_red" "$line" "$code" "$cmd" "$c_reset" >&2
  else
    printf '%s ✗ Installation failed at line %d. Re-run with --debug for details.%s\n' "$c_red" "$line" "$c_reset" >&2
  fi
}
trap '_on_error $LINENO "$BASH_COMMAND" $?' ERR
if [[ "${PP_TRACE:-0}" == 1 ]]; then
  export PS4='+ ${BASH_SOURCE##*/}:${LINENO}: '
  set -x
fi
[[ "$DEBUG" == 1 ]] && dbg "debug mode enabled"

# ---------------------------------------------------------------------------
# Interactive prompts (read from /dev/tty so this works under `curl | bash`)
# ---------------------------------------------------------------------------
ask() { # ask "Prompt" [default] -> echoes answer
  local prompt="$1" default="${2:-}" reply
  if [[ -n "$default" ]]; then
    printf '%s [%s]: ' "$prompt" "$default" >/dev/tty
  else
    printf '%s: ' "$prompt" >/dev/tty
  fi
  read -r reply </dev/tty
  printf '%s' "${reply:-$default}"
}

ask_required() { # like ask but loops until non-empty
  local val
  while :; do
    val="$(ask "$1" "${2:-}")"
    [[ -n "$val" ]] && {
      printf '%s' "$val"
      return
    }
    warn "A value is required."
  done
}

ask_prefilled() { # ask_prefilled "Prompt" "prefill" -> editable prefilled input, loops until non-empty
  local prompt="$1" prefill="$2" reply
  while :; do
    printf '%s: ' "$prompt" >/dev/tty
    read -re -i "$prefill" reply </dev/tty
    [[ -n "$reply" ]] && { printf '%s' "$reply"; return; }
    warn "A value is required."
  done
}

ask_secret() { # ask_secret "Prompt" -> echoes password (confirmed twice)
  local prompt="$1" p1 p2
  while :; do
    printf '%s: ' "$prompt" >/dev/tty
    read -rs p1 </dev/tty
    printf '\n' >/dev/tty
    printf 'Confirm %s: ' "$prompt" >/dev/tty
    read -rs p2 </dev/tty
    printf '\n' >/dev/tty
    [[ -n "$p1" && "$p1" == "$p2" ]] && {
      printf '%s' "$p1"
      return
    }
    warn "Passwords are empty or do not match — try again."
  done
}

confirm() { # confirm "Prompt" -> returns 0 on yes (default No)
  local ans
  printf '%s [y/N]: ' "$1" >/dev/tty
  read -r ans </dev/tty
  [[ "$ans" =~ ^[Yy] ]]
}

confirm_yes() { # confirm_yes "Prompt" -> returns 0 on yes (default Yes)
  local ans
  printf '%s [Y/n]: ' "$1" >/dev/tty
  read -r ans </dev/tty
  [[ ! "$ans" =~ ^[Nn] ]]
}

# ---------------------------------------------------------------------------
# Banner
# ---------------------------------------------------------------------------
clear 2>/dev/null || true
# Colour aliases for the banner only
_F="${c_blue}${c_bold}"   # frame (blue, bold — complementary to orange)
_O="${c_orange}" # Pi-Player art (orange — no bold, matches the ==> prompt colour)
_P="${c_green}"           # play button (green)
_S="${c_lgrey}${c_dim}"   # subtitle (light grey, dim)
_R="${c_reset}"

printf '\n'
printf '%s  ╔════════════════════════════════════════════════════════╗%s\n' "$_F" "$_R"
printf '%s  ║%s                                                        %s║%s\n' "$_F" "$_R" "$_F" "$_R"
printf '%s  ║%s  %s ____  _          ____  _                 %s[ ▶ ]%s       %s║%s\n' "$_F" "$_R" "$_O" "$_P" "$_R" "$_F" "$_R"
printf '%s  ║%s  %s|  _ \\(_)        |  _ \\| | __ _ _   _  ___ _ __%s       ║%s\n' "$_F" "$_R" "$_O" "$_F$_R" "$_R"
printf '%s  ║%s  %s| |_) | | ______ | |_) | |/ _ \| | | |/ _ \\ '"'"'__|%s      ║%s\n' "$_F" "$_R" "$_O" "$_F$_R" "$_R"
printf '%s  ║%s  %s|  __/| ||______||  __/| | (_| | |_| |  __/ |%s         ║%s\n' "$_F" "$_R" "$_O" "$_F$_R" "$_R"
printf '%s  ║%s  %s|_|   |_|        |_|   |_|\\__,_|\\__, |\\___|_|%s         ║%s\n' "$_F" "$_R" "$_O" "$_F$_R" "$_R"
printf '%s  ║%s  %s                                |___/%s                 ║%s\n' "$_F" "$_R" "$_O" "$_F$_R" "$_R"
printf '%s  ║%s                                                        %s║%s\n' "$_F" "$_R" "$_F" "$_R"
printf '%s  ║%s  %sArch Linux  ·  Kiosk Installer%s                        %s║%s\n' "$_F" "$_R" "$_S" "$_R" "$_F" "$_R"
printf '%s  ╚═══════════════════════════╦════════════════════════════╝%s\n' "$_F" "$_R"
printf '%s                              ║%s\n' "$_F" "$_R"
printf '%s                        ══════╩══════%s\n\n' "$_F" "$_R"
unset _F _O _P _S _R

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
info "Running pre-flight checks..."

dbg "EUID=$EUID (need 0)"
[[ $EUID -eq 0 ]] || die "Must run as root (you are on the Arch ISO, so you already are)."

dbg "checking UEFI marker: /sys/firmware/efi/efivars"
[[ -d /sys/firmware/efi/efivars ]] || die "Not booted in UEFI mode. This installer requires UEFI."

# Connectivity check. ICMP often fails under QEMU user-mode networking, so we
# treat ping as best-effort and fall back to an HTTPS fetch. Every probe has a
# hard timeout so this step can never hang the install.
info "Checking internet connectivity..."
dbg "resolver config (/etc/resolv.conf):"
[[ "$DEBUG" == 1 ]] && { grep -v '^\s*#' /etc/resolv.conf 2>/dev/null | sed 's/^/  [dbg]   /' >&2 || true; }

net_ok=0
dbg "probe 1: timeout 8 ping -c1 -W3 archlinux.org"
if timeout 8 ping -c1 -W3 archlinux.org >/dev/null 2>&1; then
  dbg "ping OK"
  net_ok=1
else
  dbg "ping failed/unsupported (normal under QEMU SLIRP) — falling back to curl"
  dbg "probe 2: curl -fsS --max-time 10 https://archlinux.org"
  if curl_out="$(curl -fsS --max-time 10 -o /dev/null https://archlinux.org 2>&1)"; then
    dbg "curl https OK"
    net_ok=1
  else
    dbg "curl https failed: ${curl_out:-<no output>}"
    dbg "probe 3: curl --max-time 10 https://1.1.1.1 (DNS-free reachability test)"
    if curl -fsS --max-time 10 -o /dev/null https://1.1.1.1 2>/dev/null; then
      dbg "raw IP 1.1.1.1 reachable but archlinux.org failed -> likely a DNS problem"
    else
      dbg "no outbound HTTPS at all -> link/NAT/gateway problem"
    fi
  fi
fi
[[ "$net_ok" == 1 ]] || die "No internet connection. Connect first (ethernet should be automatic; for wifi use 'iwctl'). Re-run with --debug for details."
dbg "connectivity OK"

dbg "syncing clock: timedatectl set-ntp true"
timedatectl set-ntp true >/dev/null 2>&1 || dbg "timedatectl set-ntp failed (non-fatal)"

# Geolocation runs here, after connectivity is confirmed, so the result is
# ready before configuration prompts begin.
info "Detecting location-based settings from your IP address..."
GEO="$(curl -fsS --max-time 6 https://ipapi.co/json/ 2>/dev/null || true)"
[[ -n "$GEO" ]] || warn "Geolocation lookup failed — using Arch defaults (UTC, en_US.UTF-8, us keymap)."
dbg "GEO response: ${GEO:-(empty)}"
geo_field() { printf '%s' "$GEO" | grep -oP "\"$1\"\s*:\s*\"?\K[^\",}]*" | head -n1 || true; }
GEO_TZ="$(geo_field timezone)"
GEO_CC="$(geo_field country_code)"
GEO_LANG="$(geo_field languages | cut -d, -f1)"

# Mirror selection runs here so pacstrap uses the fastest mirrors from the start.
sed -i 's/^#ParallelDownloads.*/ParallelDownloads = 5/' /etc/pacman.conf || true
if command -v reflector >/dev/null && [[ -n "$GEO_CC" ]]; then
  info "Selecting fastest mirrors for country '$GEO_CC' with reflector..."
  reflector --country "$GEO_CC" --age 12 --protocol https --sort rate \
    --save /etc/pacman.d/mirrorlist 2>/dev/null ||
    warn "reflector failed for '$GEO_CC'; using the ISO's default mirrorlist."
else
  warn "Skipping mirror selection (reflector unavailable or country undetected)."
fi

ok "Pre-flight checks passed."

# ---------------------------------------------------------------------------
# Gather configuration
# ---------------------------------------------------------------------------
info "Configuration"

HOSTNAME="$(ask_required "Hostname" "pi-player")"
USERNAME="$(ask_required "Username" "pi")"
USERPASS="$(ask_secret "Password for user '$USERNAME'")"
if confirm "Use the same password for root?"; then
  ROOTPASS="$USERPASS"
else
  ROOTPASS="$(ask_secret "Root password")"
fi

# Timezone — fall back to UTC if detection fails or is invalid.
TIMEZONE="$GEO_TZ"
[[ -n "$TIMEZONE" && -f "/usr/share/zoneinfo/$TIMEZONE" ]] || TIMEZONE="UTC"

# Locale — derive from the primary IP language ("en-ZA" -> "en_ZA.UTF-8").
# Fall back to the country code ("ZA" -> "en_ZA.UTF-8") when the language
# field is missing or lacks a region subtag, then fall back to en_US.
if [[ "$GEO_LANG" =~ ^[a-z]{2}-[A-Z]{2}$ ]]; then
  LOCALE="${GEO_LANG/-/_}.UTF-8"
elif [[ -n "$GEO_CC" ]]; then
  LOCALE="en_${GEO_CC}.UTF-8"
else
  LOCALE="en_US.UTF-8"
fi

# Console keymap — map the country code to a keymap (most default to "us").
case "$GEO_CC" in
GB | IE) KEYMAP="uk" ;;
DE | AT | CH) KEYMAP="de" ;;
FR) KEYMAP="fr" ;;
ES) KEYMAP="es" ;;
IT) KEYMAP="it" ;;
PT) KEYMAP="pt-latin1" ;;
BR) KEYMAP="br-abnt2" ;;
*) KEYMAP="us" ;;
esac

ok "Detected: timezone=$TIMEZONE  locale=$LOCALE  keymap=$KEYMAP  country=${GEO_CC:-?}"

# --- Network ---------------------------------------------------------------
# Detect the first wired interface as a sensible default.
DEFAULT_IFACE="$(ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | grep -E '^(en|eth)' | head -n1 || true)"

# Probe the live environment for DHCP-assigned settings to use as defaults
# when the user switches to a static IP configuration.
CURRENT_IP=""
CURRENT_GW=""
CURRENT_DNS=""
CURRENT_DOMAIN=""
if [[ -n "$DEFAULT_IFACE" ]]; then
  CURRENT_IP="$(ip -4 addr show "$DEFAULT_IFACE" 2>/dev/null \
    | grep -oP '(?<=inet )\d+\.\d+\.\d+\.\d+/\d+' | head -n1 || true)"
fi
CURRENT_GW="$(ip route show default 2>/dev/null \
  | grep -oP '(?<=via )\S+' | head -n1 || true)"
# Prefer resolvectl (shows actual DHCP-given servers, not the stub resolver)
CURRENT_DNS="$(resolvectl dns "${DEFAULT_IFACE:-}" 2>/dev/null \
  | grep -oP '\b(?!127\.)\d+\.\d+\.\d+\.\d+\b' | head -n2 | tr '\n' ' ' | sed 's/ *$//' || true)"
[[ -z "$CURRENT_DNS" ]] && \
  CURRENT_DNS="$(awk '/^nameserver/ && $2 !~ /^127\./{print $2}' /etc/resolv.conf 2>/dev/null \
    | head -n2 | tr '\n' ' ' | sed 's/ *$//' || true)"
# Search domain — try resolvectl per-interface first (most accurate for DHCP),
# then fall back to the search line in resolv.conf (covers dhcpcd / stub-resolver)
CURRENT_DOMAIN=""
if [[ -n "$DEFAULT_IFACE" ]]; then
  CURRENT_DOMAIN="$(resolvectl status "$DEFAULT_IFACE" 2>/dev/null \
    | grep -oP '(?<=DNS Domain: )\S+' | grep -v '^[~.]' | head -n1 || true)"
fi
# Fallback: systemd-networkd lease file (DHCP option 15, present on live ISO)
if [[ -z "$CURRENT_DOMAIN" && -n "$DEFAULT_IFACE" ]]; then
  _ifindex="$(ip link show "$DEFAULT_IFACE" 2>/dev/null | awk 'NR==1{sub(/:$/,"",$1); print $1}')"
  CURRENT_DOMAIN="$(awk -F= '/^(DOMAIN|DOMAINNAME)=/{print $2; exit}' \
    "/run/systemd/netif/leases/${_ifindex:-_}" 2>/dev/null || true)"
fi
[[ -z "$CURRENT_DOMAIN" ]] && \
  CURRENT_DOMAIN="$(awk '/^(search|domain)/{for(i=2;i<=NF;i++) if($i !~ /^[~.]/) {print $i; exit}}' \
    /etc/resolv.conf 2>/dev/null || true)"
dbg "current IP=${CURRENT_IP:-?}  gw=${CURRENT_GW:-?}  dns=${CURRENT_DNS:-?}  domain=${CURRENT_DOMAIN:-?}"

NET_TYPE="dhcp"
IFACE="" STATIC_ADDR="" STATIC_GW="" STATIC_DNS="" STATIC_DOMAIN=""
if confirm "Use DHCP for networking? (No = configure a static IP)"; then
  NET_TYPE="dhcp"
else
  NET_TYPE="static"
  IFACE="$(ask_required "Interface name" "${DEFAULT_IFACE:-eth0}")"
  STATIC_ADDR="$(ask_required "Static address (CIDR)" "${CURRENT_IP:-192.168.1.50/24}")"
  STATIC_GW="$(ask_required "Gateway" "${CURRENT_GW:-192.168.1.1}")"
  STATIC_DNS="$(ask "DNS servers (space-separated)" "${CURRENT_DNS:-1.1.1.1 8.8.8.8}")"
  STATIC_DOMAIN="$(ask "DNS search domain (e.g. local, office.example.com)" "${CURRENT_DOMAIN:-}")"
fi

# --- Network mount (Samba/SMB) ---------------------------------------------
MOUNT_ENABLED=0
MOUNT_ADDR="" MOUNT_USER="" MOUNT_PASS="" MOUNT_DOMAIN="" MOUNT_POINT="" PIPLAYER_DIR=""
if confirm "Add a network (Samba/SMB) share mount?"; then
  MOUNT_ENABLED=1
  MOUNT_ADDR="$(ask_required "Share address (e.g. //fileserver/media)")"

  # Default mount point: /mnt/<last path segment of the share address>
  _mount_dir="$(printf '%s' "$MOUNT_ADDR" \
    | sed 's|^smb:||; s|^//||' | tr '/' '\n' | grep -v '^$' | tail -n1)"

  MOUNT_USER="$(ask_required "Share username" "$USERNAME")"
  if confirm_yes "Use the same password as '$USERNAME' for the share?"; then
    MOUNT_PASS="$USERPASS"
  else
    MOUNT_PASS="$(ask_secret "Share password")"
  fi

  # Try to detect the Windows domain from the DHCP-provided search domain.
  _dhcp_domain="$(awk '/^(domain|search)/{print $2; exit}' /etc/resolv.conf 2>/dev/null || true)"
  [[ -z "$_dhcp_domain" ]] && \
    _dhcp_domain="$(resolvectl status "${DEFAULT_IFACE:-}" 2>/dev/null \
      | grep -oP '(?<=DNS Domain: )\S+' | grep -v '^\.' | head -n1 || true)"
  MOUNT_DOMAIN="$(ask "Windows domain (leave blank if not required)" "${_dhcp_domain:-}")"

  MOUNT_POINT="$(ask_required "Mount point" "/mnt/${_mount_dir:-network}")"

  # Pre-fill with the mount point — press Enter to accept, or edit to point at a subfolder.
  PIPLAYER_DIR="$(ask_prefilled "Pi-player media directory" "$MOUNT_POINT")"
else
  PIPLAYER_DIR=""
fi

# --- Target disk -----------------------------------------------------------
DEFAULT_DISK="$(lsblk -dpno NAME,TYPE | awk '$2=="disk"{print $1; exit}')"
mapfile -t DISK_LINES < <(lsblk -dpno NAME,SIZE,MODEL | grep -vE 'loop|sr0')
dbg "disks found: ${#DISK_LINES[@]} — ${DISK_LINES[*]:-(none)}"
[[ ${#DISK_LINES[@]} -gt 0 ]] || die "No disks found. Check that your disk is connected and visible (lsblk)."
info "Available disks (ALL DATA on the selected disk will be erased):"
printf '\n' >/dev/tty
PS3=$'\nSelect disk number: '
select _line in "${DISK_LINES[@]}"; do
  [[ -n "$_line" ]] && {
    DISK="$(awk '{print $1}' <<<"$_line")"
    break
  }
  warn "Invalid selection — enter a number from the list."
done </dev/tty >/dev/tty
[[ -b "$DISK" ]] || die "Not a block device: $DISK"

# Partition path suffix differs for nvme/mmc (p1) vs sata/scsi (1).
if [[ "$DISK" =~ [0-9]$ ]]; then PSEP="p"; else PSEP=""; fi
ESP_PART="${DISK}${PSEP}1"
ROOT_PART="${DISK}${PSEP}2"

# CPU microcode
case "$(grep -m1 -o -E 'GenuineIntel|AuthenticAMD' /proc/cpuinfo || true)" in
GenuineIntel) UCODE="intel-ucode" ;;
AuthenticAMD) UCODE="amd-ucode" ;;
*) UCODE="" ;;
esac

# ---------------------------------------------------------------------------
# Summary + final confirmation (last chance before destruction)
# ---------------------------------------------------------------------------
cat >/dev/tty <<SUMMARY

${c_orange}========================= INSTALL SUMMARY =========================${c_reset}
  Hostname     : $HOSTNAME
  Username     : $USERNAME (sudo)
  Locale       : $LOCALE      Keymap: $KEYMAP   (auto-detected)
  Timezone     : $TIMEZONE   (auto-detected)
  Mirrors      : country ${GEO_CC:-unknown} via reflector
  Microcode    : ${UCODE:-none detected}
  Network      : $NET_TYPE${IFACE:+  iface=$IFACE}${STATIC_ADDR:+  addr=$STATIC_ADDR gw=$STATIC_GW}${STATIC_DOMAIN:+  search=$STATIC_DOMAIN}
  SMB mount    : ${MOUNT_ADDR:+$MOUNT_ADDR  ->  $MOUNT_POINT  (user: $MOUNT_USER${MOUNT_DOMAIN:+  domain: $MOUNT_DOMAIN})}${MOUNT_ADDR:-none}
  Pi-Player dir: ${PIPLAYER_DIR:-n/a}
  Target disk  : $DISK  ->  ESP=$ESP_PART  root=$ROOT_PART
  Testing mode : ${TESTING/0/no}${TESTING/1/yes (spice-vdagent for clipboard)}
${c_red}  ALL DATA ON $DISK WILL BE PERMANENTLY ERASED.${c_reset}
${c_orange}===================================================================${c_reset}

SUMMARY

confirm "Proceed with installation?" || die "Aborted by user."

# ---------------------------------------------------------------------------
# Partition, format, mount
# ---------------------------------------------------------------------------
info "Partitioning $DISK..."
wipefs -af "$DISK"
sgdisk --zap-all "$DISK"
sgdisk -n 1:1MiB:+1GiB -t 1:ef00 -c 1:EFI "$DISK"
sgdisk -n 2:0:0 -t 2:8300 -c 2:root "$DISK"
partprobe "$DISK"
sleep 2

info "Creating filesystems..."
mkfs.fat -F32 -n EFI "$ESP_PART"
mkfs.btrfs -f -L root "$ROOT_PART"

info "Creating btrfs subvolumes..."
mount "$ROOT_PART" /mnt
btrfs subvolume create /mnt/@
btrfs subvolume create /mnt/@home
btrfs subvolume create /mnt/@log
btrfs subvolume create /mnt/@pkg
umount /mnt

BTRFS_OPTS="noatime,compress=zstd,ssd,discard=async"
mount -o "$BTRFS_OPTS,subvol=@" "$ROOT_PART" /mnt
mkdir -p /mnt/{home,var/log,var/cache/pacman/pkg,boot}
mount -o "$BTRFS_OPTS,subvol=@home" "$ROOT_PART" /mnt/home
mount -o "$BTRFS_OPTS,subvol=@log" "$ROOT_PART" /mnt/var/log
mount -o "$BTRFS_OPTS,subvol=@pkg" "$ROOT_PART" /mnt/var/cache/pacman/pkg
mount "$ESP_PART" /mnt/boot

# ---------------------------------------------------------------------------
# Install base system
# ---------------------------------------------------------------------------
info "Installing base system (pacstrap)... this can take a while."
TESTING_PKGS=()
[[ "$TESTING" == 1 ]] && TESTING_PKGS=(spice-vdagent)
pacstrap -K /mnt \
  base linux linux-firmware ${UCODE} \
  btrfs-progs sudo git vim ansible \
  openssh python \
  zram-generator ufw \
  pipewire pipewire-pulse pipewire-alsa wireplumber \
  efibootmgr \
  "${TESTING_PKGS[@]}"

info "Generating fstab..."
genfstab -U /mnt >>/mnt/etc/fstab

# ---------------------------------------------------------------------------
# Network configuration (written directly into the target)
# ---------------------------------------------------------------------------
info "Writing systemd-networkd configuration ($NET_TYPE)..."
mkdir -p /mnt/etc/systemd/network
if [[ "$NET_TYPE" == "dhcp" ]]; then
  cat >/mnt/etc/systemd/network/20-ethernet.network <<'NETEOF'
[Match]
# Match by interface-name glob (not Type=ether) to avoid matching veth* in containers.
# https://bugs.archlinux.org/task/70892
Name=en*
Name=eth*

[Link]
RequiredForOnline=routable

[Network]
DHCP=yes
MulticastDNS=yes
UseDomains=true

[DHCPv4]
RouteMetric=100

[IPv6AcceptRA]
RouteMetric=100
NETEOF
else
  {
    printf '[Match]\nName=%s\n\n' "$IFACE"
    printf '[Link]\nRequiredForOnline=routable\n\n'
    printf '[Network]\nAddress=%s\nGateway=%s\n' "$STATIC_ADDR" "$STATIC_GW"
    for d in $STATIC_DNS; do printf 'DNS=%s\n' "$d"; done
    [[ -n "$STATIC_DOMAIN" ]] && printf 'Domains=%s\n' "$STATIC_DOMAIN"
  } >/mnt/etc/systemd/network/20-static.network
fi

# Continue boot as soon as ONE interface is online (eth1 / eth2 / wifi),
# instead of blocking ~2min for every routable interface and timing out.
mkdir -p /mnt/etc/systemd/system/systemd-networkd-wait-online.service.d
cat >/mnt/etc/systemd/system/systemd-networkd-wait-online.service.d/any.conf <<'WAITEOF'
# Only runs at boot when something pulls in network-online.target (e.g. the SMB
# mount). `--any` makes it succeed as soon as the first interface is routable.
[Service]
ExecStart=
ExecStart=/usr/lib/systemd/systemd-networkd-wait-online --any
WAITEOF

# ---------------------------------------------------------------------------
# Configure the installed system inside chroot
# ---------------------------------------------------------------------------
info "Configuring the installed system..."
# Export everything the chroot script needs. Secrets travel via the environment
# (inherited through arch-chroot) so they never appear in a here-doc or argv.
export PP_HOSTNAME="$HOSTNAME" PP_USERNAME="$USERNAME" PP_TIMEZONE="$TIMEZONE" \
  PP_LOCALE="$LOCALE" PP_KEYMAP="$KEYMAP" PP_ROOTPASS="$ROOTPASS" PP_USERPASS="$USERPASS" \
  PP_TESTING="$TESTING"

# Filesystem UUID of the btrfs root, needed for the UKI kernel cmdline.
export PP_ROOT_UUID="$(blkid -s UUID -o value "$ROOT_PART")"

arch-chroot /mnt /bin/bash -euo pipefail <<'CHROOT'
# --- time -------------------------------------------------------------------
ln -sf "/usr/share/zoneinfo/$PP_TIMEZONE" /etc/localtime
hwclock --systohc

# --- locale & keymap --------------------------------------------------------
sed -i "s/^#\s*\(${PP_LOCALE//./\\.}\b.*\)/\1/" /etc/locale.gen
sed -i 's/^#\s*\(en_US\.UTF-8\b.*\)/\1/'        /etc/locale.gen
locale-gen
echo "LANG=$PP_LOCALE"   > /etc/locale.conf
echo "KEYMAP=$PP_KEYMAP" > /etc/vconsole.conf

# --- hostname ---------------------------------------------------------------
echo "$PP_HOSTNAME" > /etc/hostname
cat > /etc/hosts <<HOSTS
127.0.0.1   localhost
::1         localhost
127.0.1.1   $PP_HOSTNAME.localdomain $PP_HOSTNAME
HOSTS

# --- pacman cosmetics -------------------------------------------------------
sed -i 's/^#Color/Color/'                        /etc/pacman.conf
sed -i 's/^#ParallelDownloads.*/ParallelDownloads = 5/' /etc/pacman.conf

# --- zram swap (zstd) -------------------------------------------------------
cat > /etc/systemd/zram-generator.conf <<ZRAM
[zram0]
zram-size = min(ram, 8192)
compression-algorithm = zstd
ZRAM

# --- UKI + mkinitcpio -------------------------------------------------------
echo "root=UUID=$PP_ROOT_UUID rootflags=subvol=@ rw" > /etc/kernel/cmdline
mkdir -p /boot/EFI/Linux
preset=/etc/mkinitcpio.d/linux.preset
# Build a single unified kernel image; disable the split image/fallback outputs.
sed -i 's|^#\?\s*default_image=.*|#default_image="/boot/initramfs-linux.img"|'                 "$preset"
sed -i 's|^#\?\s*fallback_image=.*|#fallback_image="/boot/initramfs-linux-fallback.img"|'        "$preset"
if grep -q '^#\?\s*default_uki=' "$preset"; then
  sed -i 's|^#\?\s*default_uki=.*|default_uki="/boot/EFI/Linux/arch-linux.efi"|' "$preset"
else
  echo 'default_uki="/boot/EFI/Linux/arch-linux.efi"' >> "$preset"
fi
sed -i "s|^PRESETS=.*|PRESETS=('default')|" "$preset"
mkinitcpio -P

# --- bootloader (systemd-boot; auto-discovers the UKI) ----------------------
bootctl install
cat > /boot/loader/loader.conf <<LOADER
default  arch-linux.efi
timeout  3
console-mode max
editor   no
LOADER

# --- users & sudo -----------------------------------------------------------
echo "root:$PP_ROOTPASS" | chpasswd
useradd -m -G wheel -s /bin/bash "$PP_USERNAME"
echo "$PP_USERNAME:$PP_USERPASS" | chpasswd
sed -i 's/^# %wheel ALL=(ALL:ALL) ALL/%wheel ALL=(ALL:ALL) ALL/' /etc/sudoers

# --- autologin --------------------------------------------------------------
mkdir -p /etc/systemd/system/getty@tty1.service.d
cat > /etc/systemd/system/getty@tty1.service.d/autologin.conf <<AUTOLOGIN
[Service]
ExecStart=
ExecStart=-/sbin/agetty -o '-p -f -- \\u' --noclear --autologin $PP_USERNAME %I \$TERM
AUTOLOGIN

# --- setup motd -------------------------------------------------------------
cat > /etc/motd <<'MOTD'

  Pi-Player setup is running in the background.
  To watch progress:  journalctl -u pi-player-setup.service -f

MOTD

# --- services ---------------------------------------------------------------
systemctl enable systemd-networkd
systemctl enable systemd-resolved
systemctl enable systemd-timesyncd
systemctl enable sshd
systemctl enable ufw
if [[ "$PP_TESTING" == 1 ]]; then systemctl enable spice-vdagentd; fi
CHROOT

# arch-chroot bind-mounts a tmpfs onto /etc/resolv.conf inside the chroot,
# so rm fails with "Device or resource busy" if done from inside. Do it here
# against /mnt/etc/resolv.conf instead, where it's just a regular file.
rm -f /mnt/etc/resolv.conf
ln -sf /run/systemd/resolve/stub-resolv.conf /mnt/etc/resolv.conf

# ---------------------------------------------------------------------------
# Network mount config (consumed by ansible-pull on first boot)
# ---------------------------------------------------------------------------
# Per-device values + secrets can't live in the public repo that ansible-pull
# clones, so we seed them on the target here. The playbook reads the non-secret
# values as Ansible local facts and templates the .mount unit from them; the
# password lives only in the root-only credentials file below.
if [[ "$MOUNT_ENABLED" == 1 ]]; then
  info "Writing network mount configuration..."

  # Local facts: ansible reads this as ansible_local.pi_player.* (no secrets).
  mkdir -p /mnt/etc/ansible/facts.d
  cat >/mnt/etc/ansible/facts.d/pi_player.fact <<FACTS
{
  "mount_enabled": true,
  "mount_what": "$MOUNT_ADDR",
  "mount_where": "$MOUNT_POINT",
  "mount_user": "$MOUNT_USER",
  "mount_domain": "$MOUNT_DOMAIN",
  "piplayer_dir": "$PIPLAYER_DIR"
}
FACTS
  chmod 0644 /mnt/etc/ansible/facts.d/pi_player.fact

  # CIFS credentials — root-only, never enters git or any ansible-readable var.
  mkdir -p /mnt/etc/samba
  {
    printf 'username=%s\n' "$MOUNT_USER"
    printf 'password=%s\n' "$MOUNT_PASS"
    [[ -n "$MOUNT_DOMAIN" ]] && printf 'domain=%s\n' "$MOUNT_DOMAIN"
  } >/mnt/etc/samba/credentials
  chmod 0600 /mnt/etc/samba/credentials
else
  # Record that no mount was requested so the playbook skips the section.
  mkdir -p /mnt/etc/ansible/facts.d
  printf '{ "mount_enabled": false }\n' >/mnt/etc/ansible/facts.d/pi_player.fact
  chmod 0644 /mnt/etc/ansible/facts.d/pi_player.fact
fi

# ---------------------------------------------------------------------------
# First-boot setup service (runs ansible-pull as root on first boot)
# ---------------------------------------------------------------------------
info "Writing first-boot setup service..."
cat >/mnt/etc/systemd/system/pi-player-setup.service <<EOF
[Unit]
Description=Pi-Player initial setup (ansible-pull)
After=network-online.target
Wants=network-online.target
ConditionPathExists=!/etc/pi-player-setup-done

[Service]
Type=oneshot
ExecStartPre=/usr/bin/ansible-galaxy collection install kewlfft.aur
ExecStart=/usr/bin/ansible-pull -U https://github.com/rivers-church/pi-player --extra-vars "pi_user=$USERNAME" ansible/playbook.yml
ExecStartPost=/usr/bin/touch /etc/pi-player-setup-done
ExecStartPost=-/usr/bin/systemctl disable pi-player-setup.service
ExecStartPost=-/usr/bin/rm /etc/systemd/system/pi-player-setup.service
ExecStartPost=/usr/bin/systemctl reboot
RemainAfterExit=yes
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

mkdir -p /mnt/etc/systemd/system/multi-user.target.wants
ln -sf /etc/systemd/system/pi-player-setup.service \
  /mnt/etc/systemd/system/multi-user.target.wants/pi-player-setup.service

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
ok "Installation complete. Rebooting into new system — ansible-pull will complete setup on first boot."

umount -R /mnt 2>/dev/null || true
systemctl reboot
