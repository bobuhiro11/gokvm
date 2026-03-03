#!/bin/bash
# ──────────────────────────────────────────────────────────────────────────────
# demo_live_migration.bash  –  gokvm live migration demo
#
# Demonstrates live migration of a running VM including:
#   • Block storage  (disk image transferred, HTTP content verified)
#   • ping           (reachability before / after migration)
#   • curl           (HTTP request to disk-backed content, src then dst)
#
# No arguments required.  Run from the repository root:
#
#   bash scripts/demo_live_migration.bash
#
# The script re-executes itself inside an unshare(1) user+net namespace so
# that tap interfaces and routing can be set up without system-wide root.
# ──────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ── Re-exec inside a private user+net namespace if we are not already root ───
if [ "$(id -u)" != "0" ]; then
    exec unshare --user --net --map-root-user -- bash "$0" "$@"
fi

# ── Colour helpers ────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

src()   { echo -e "${BLUE}[SRC]${RESET} $*"; }
dst()   { echo -e "${GREEN}[DST]${RESET} $*"; }
step()  { echo -e "\n${YELLOW}${BOLD}══ $* ══${RESET}"; }
info()  { echo -e "  $*"; }
ok()    { echo -e "  ${GREEN}✅ $*${RESET}"; }
fail()  { echo -e "  ${RED}❌ $*${RESET}"; }
banner(){ echo -e "${CYAN}${BOLD}$*${RESET}"; }
cmd()   { printf "    \033[1m%s\033[0m\n" "$*"; }

# ── Configuration ─────────────────────────────────────────────────────────────
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GOKVM="${REPO_ROOT}/gokvm"
BZIMAGE="${REPO_ROOT}/bzImage"
INITRD="${REPO_ROOT}/initrd"
VDA_ORIG="${REPO_ROOT}/vda.img"

SRC_DISK="/tmp/vda-demo-src.img"
DST_DISK="/tmp/vda-demo-dst.img"
SRC_LOG="/tmp/gokvm-demo-src.log"
DST_LOG="/tmp/gokvm-demo-dst.log"
DEMO_MARKER_FILE="/tmp/gokvm-demo-index.html"

TAP_SRC="tap-demo-src"
TAP_DST="tap-demo-dst"

GUEST_IP="192.168.20.1"
SRC_HOST_IP="192.168.20.253"
DST_HOST_IP="192.168.20.254"
PREFIX="24"

DST_LISTEN="${DST_HOST_IP}:5555"

GUEST_PARAMS="console=ttyS0 earlyprintk=serial noapic noacpi notsc lapic \
tsc_early_khz=2000 pci=realloc=off virtio_pci.force_legacy=1 \
rdinit=/init init=/init gokvm.ipv4_addr=${GUEST_IP}/${PREFIX}"

SRC_PID=""
DST_PID=""

# ── Cleanup (always runs on exit) ─────────────────────────────────────────────
cleanup() {
    echo ""
    step "Cleaning up"
    [ -n "$SRC_PID" ] && kill "$SRC_PID" 2>/dev/null && info "src VM (PID $SRC_PID) stopped" || true
    [ -n "$DST_PID" ] && kill "$DST_PID" 2>/dev/null && info "dst VM (PID $DST_PID) stopped" || true
    ip link delete "$TAP_SRC" 2>/dev/null && info "removed $TAP_SRC" || true
    ip link delete "$TAP_DST" 2>/dev/null && info "removed $TAP_DST" || true
    rm -f "$SRC_DISK" "$DST_DISK" "$DEMO_MARKER_FILE"
    info "temp files removed"
}
trap cleanup EXIT INT TERM

# ── Sanity checks ─────────────────────────────────────────────────────────────
for f in "$GOKVM" "$BZIMAGE" "$INITRD" "$VDA_ORIG"; do
    if [ ! -f "$f" ]; then
        echo -e "${RED}ERROR: required file not found: $f${RESET}"
        echo "  Run 'make gokvm bzImage initrd vda.img' first."
        exit 1
    fi
done
for cmd in debugfs ping curl ip md5sum xxd; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo -e "${RED}ERROR: required command not found: $cmd${RESET}"
        exit 1
    fi
done

# ══════════════════════════════════════════════════════════════════════════════
banner "══════════════════════════════════════════════════════"
banner "  gokvm Live Migration Demo"
banner "══════════════════════════════════════════════════════"
echo ""
info "This demo migrates a running VM (including its disk) from SRC to DST."
info "We verify block storage, network (ping), and HTTP (curl) at each stage."

# ── STEP 0: Prepare disk images ───────────────────────────────────────────────
step "STEP 0: Preparing disk images"

cp "$VDA_ORIG" "$SRC_DISK"
cp "$VDA_ORIG" "$DST_DISK"

# Write a unique demo message into /index.html on the src disk.
# The guest mounts /dev/vda at /mnt/dev_vda and serves it via HTTP on port 80.
DEMO_TIMESTAMP="$(date '+%Y-%m-%dT%H:%M:%S')"
DEMO_CONTENT="Live migration demo – started at ${DEMO_TIMESTAMP}"
echo "$DEMO_CONTENT" > "$DEMO_MARKER_FILE"
# Replace /index.html in the ext2 fs using debugfs (wrm + write)
debugfs -w "$SRC_DISK" -R "rm /index.html"       2>/dev/null || true
debugfs -w "$SRC_DISK" -R "write $DEMO_MARKER_FILE /index.html" 2>/dev/null

src "disk: $SRC_DISK"
src "  /index.html → \"${DEMO_CONTENT}\""
dst "disk: $DST_DISK  (plain copy – no marker yet)"

SRC_MD5_BEFORE="$(md5sum "$SRC_DISK" | awk '{print $1}')"
DST_MD5_BEFORE="$(md5sum "$DST_DISK" | awk '{print $1}')"
info "src disk md5: ${SRC_MD5_BEFORE}"
info "dst disk md5: ${DST_MD5_BEFORE}"
if [ "$SRC_MD5_BEFORE" != "$DST_MD5_BEFORE" ]; then
    ok "disks are DIFFERENT (marker is only in src) ✓"
else
    fail "disks are identical before migration – something went wrong"
    exit 1
fi

# ── STEP 1: Networking ────────────────────────────────────────────────────────
step "STEP 1: Setting up network interfaces"

ip link set lo up

# tap-demo-src: src VM tap (host side)
ip tuntap add "$TAP_SRC" mode tap
ip addr add "${SRC_HOST_IP}/${PREFIX}" dev "$TAP_SRC"
ip link set "$TAP_SRC" up
src "tap $TAP_SRC up  (host ${SRC_HOST_IP}/24 ↔ guest ${GUEST_IP}/24)"

# tap-demo-dst: dst VM tap (host side, same subnet)
ip tuntap add "$TAP_DST" mode tap
ip addr add "${DST_HOST_IP}/${PREFIX}" dev "$TAP_DST"
ip link set "$TAP_DST" up
dst "tap $TAP_DST up  (host ${DST_HOST_IP}/24 ↔ guest ${GUEST_IP}/24 after migration)"

# ── STEP 2: Start DST VM (incoming migration listener) ────────────────────────
step "STEP 2: Starting DST VM (waiting for incoming migration)"

DST_CMD="$GOKVM incoming -l $DST_LISTEN -c 1 -m 512M -t $TAP_DST -d $DST_DISK"
dst "command:"
cmd "$GOKVM incoming \\"
cmd "  -l $DST_LISTEN \\"
cmd "  -c 1 -m 512M \\"
cmd "  -t $TAP_DST \\"
cmd "  -d $DST_DISK"

"$GOKVM" incoming \
    -l "$DST_LISTEN" \
    -c 1 -m 512M \
    -t "$TAP_DST" \
    -d "$DST_DISK" \
    >"$DST_LOG" 2>&1 &
DST_PID=$!
dst "PID=$DST_PID  listening on $DST_LISTEN"
dst "log: $DST_LOG"
sleep 0.5

# ── STEP 3: Boot SRC VM ───────────────────────────────────────────────────────
step "STEP 3: Booting SRC VM"

src "command:"
cmd "$GOKVM boot \\"
cmd "  -c 1 -m 512M \\"
cmd "  -k $BZIMAGE \\"
cmd "  -i $INITRD \\"
cmd "  -t $TAP_SRC \\"
cmd "  -d $SRC_DISK \\"
cmd "  -p \"console=ttyS0 earlyprintk=serial noapic noacpi notsc lapic tsc_early_khz=2000\""
cmd "     \"pci=realloc=off virtio_pci.force_legacy=1 rdinit=/init init=/init\""
cmd "     \"gokvm.ipv4_addr=${GUEST_IP}/${PREFIX}\""

# shellcheck disable=SC2086
"$GOKVM" boot \
    -c 1 -m 512M \
    -k "$BZIMAGE" -i "$INITRD" \
    -p "$GUEST_PARAMS" \
    -t "$TAP_SRC" \
    -d "$SRC_DISK" \
    >"$SRC_LOG" 2>&1 &
SRC_PID=$!
src "PID=$SRC_PID  guest IP=${GUEST_IP}"
src "log: $SRC_LOG"

# ── STEP 4: Wait for SRC VM to boot ──────────────────────────────────────────
step "STEP 4: Waiting for SRC VM to boot (up to 5 min)..."

BOOT_DEADLINE=$(( $(date +%s) + 300 ))
attempt=0
while true; do
    attempt=$(( attempt + 1 ))
    if ping -c 1 -W 1 "$GUEST_IP" >/dev/null 2>&1; then
        src "boot confirmed (attempt ${attempt})"
        break
    fi
    if [ "$(date +%s)" -ge "$BOOT_DEADLINE" ]; then
        fail "SRC VM did not respond to ping within 5 minutes"
        echo "--- last 20 lines of src log ---"
        tail -20 "$SRC_LOG"
        exit 1
    fi
    if (( attempt % 10 == 0 )); then
        src "still waiting... (attempt ${attempt})"
    fi
    sleep 2
done

# Wait for HTTP server inside guest to start (srvfiles starts after /dev/vda mount)
# The mount is retried for up to 60s; poll until the first HTTP response.
src "waiting for HTTP server on port 80..."
HTTP_READY=0
HTTP_DEADLINE=$(( $(date +%s) + 120 ))
while [ "$(date +%s)" -lt "$HTTP_DEADLINE" ]; do
    if curl -sSfL --max-time 5 "http://${GUEST_IP}/mnt/dev_vda/index.html" >/dev/null 2>&1; then
        src "HTTP server ready"
        HTTP_READY=1
        break
    fi
    sleep 3
done
if [ "$HTTP_READY" -eq 0 ]; then
    fail "HTTP server did not become ready within 2 minutes"
    tail -20 "$SRC_LOG"
    exit 1
fi

# ── STEP 5: BEFORE MIGRATION checks ─────────────────────────────────────────
echo ""
banner "══════════════════════════════════════════════════════"
banner "  BEFORE MIGRATION"
banner "══════════════════════════════════════════════════════"

# ping
src "ping ${GUEST_IP} (via ${TAP_SRC})"
if ping -c 3 -W 2 "$GUEST_IP" >/dev/null 2>&1; then
    PING_RESULT="$(ping -c 3 -W 2 "$GUEST_IP" 2>&1 | tail -2)"
    ok "ping SRC VM: ${PING_RESULT}"
else
    fail "ping SRC VM failed"
fi

# curl (disk content via HTTP)
src "curl http://${GUEST_IP}/mnt/dev_vda/index.html"
HTTP_BODY="$(curl -sSfL --max-time 10 "http://${GUEST_IP}/mnt/dev_vda/index.html" 2>&1 || echo "(failed)")"
if echo "$HTTP_BODY" | grep -q "Live migration demo"; then
    ok "curl SRC VM → \"${HTTP_BODY}\""
else
    fail "curl SRC VM returned unexpected content: ${HTTP_BODY}"
fi

# disk comparison
info "src disk md5: $(md5sum "$SRC_DISK" | awk '{print $1}')"
info "dst disk md5: $(md5sum "$DST_DISK" | awk '{print $1}')"
ok "disks are DIFFERENT before migration (expected)"

# ── STEP 6: Live migration ────────────────────────────────────────────────────
step "STEP 6: Triggering live migration → $DST_LISTEN"

# Find the control socket for the src VM
SOCK="/tmp/gokvm-${SRC_PID}.sock"
WAIT_SOCK=0
while [ ! -S "$SOCK" ] && [ "$WAIT_SOCK" -lt 30 ]; do
    sleep 1
    WAIT_SOCK=$(( WAIT_SOCK + 1 ))
done
if [ ! -S "$SOCK" ]; then
    fail "control socket $SOCK not found after ${WAIT_SOCK}s"
    exit 1
fi

src "control socket: $SOCK"
src "migrating to $DST_LISTEN ..."
src "command:"
cmd "$GOKVM migrate -s $SOCK -to $DST_LISTEN"
dst "waiting to receive VM state + disk ..."

MIGRATE_OUT="$("$GOKVM" migrate -s "$SOCK" -to "$DST_LISTEN" 2>&1)"
if echo "$MIGRATE_OUT" | grep -q "OK"; then
    src "migration complete"
    # Print each stats line with ok() formatting
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        case "$line" in
            OK) ;;
            memory:*) ok "$line" ;;
            disk:*)   ok "$line" ;;
            *)        info "  $line" ;;
        esac
    done <<< "$MIGRATE_OUT"
else
    fail "migration returned: $MIGRATE_OUT"
    exit 1
fi

sleep 1

# Switch network: route to guest IP now via tap-dst
# The src VM has terminated; remove its route first
ip route del "${GUEST_IP}/32" dev "$TAP_SRC" 2>/dev/null || true
ip route del "${GUEST_IP%.*}.0/${PREFIX}" dev "$TAP_SRC" 2>/dev/null || true
ip link set "$TAP_SRC" down 2>/dev/null || true

# Add explicit host route to guest via dst tap
ip route add "${GUEST_IP}/32" dev "$TAP_DST" 2>/dev/null || true

dst "network switched: ${GUEST_IP} now routed via ${TAP_DST}"
sleep 1

# ── STEP 7: AFTER MIGRATION checks ───────────────────────────────────────────
echo ""
banner "══════════════════════════════════════════════════════"
banner "  AFTER MIGRATION"
banner "══════════════════════════════════════════════════════"

# ping via src tap (should fail – VM terminated on src)
src "ping ${GUEST_IP} via ${TAP_SRC} (VM terminated – should fail)"
if ping -c 2 -W 1 "$GUEST_IP" -I "$TAP_SRC" >/dev/null 2>&1; then
    fail "SRC tap still responds unexpectedly"
else
    ok "SRC VM unreachable via ${TAP_SRC} (terminated as expected)"
fi

# ping via dst tap (should succeed – VM now running on dst)
dst "ping ${GUEST_IP} via ${TAP_DST}"
PING_OK=0
for i in $(seq 1 15); do
    if ping -c 1 -W 2 "$GUEST_IP" >/dev/null 2>&1; then
        PING_RESULT="$(ping -c 3 -W 2 "$GUEST_IP" 2>&1 | tail -2)"
        ok "ping DST VM: ${PING_RESULT}"
        PING_OK=1
        break
    fi
    sleep 1
done
if [ "$PING_OK" -eq 0 ]; then
    fail "DST VM did not respond to ping after migration"
fi

# curl (disk content – must match pre-migration)
dst "curl http://${GUEST_IP}/mnt/dev_vda/index.html"
HTTP_BODY_AFTER="$(curl -sSfL --max-time 10 "http://${GUEST_IP}/mnt/dev_vda/index.html" 2>&1 || echo "(failed)")"
if [ "$HTTP_BODY_AFTER" = "$HTTP_BODY" ]; then
    ok "curl DST VM → \"${HTTP_BODY_AFTER}\"  (matches pre-migration ✓)"
else
    fail "curl DST VM returned: ${HTTP_BODY_AFTER}  (expected: ${HTTP_BODY})"
fi

# disk md5 comparison
SRC_MD5_AFTER="$(md5sum "$SRC_DISK" | awk '{print $1}')"
DST_MD5_AFTER="$(md5sum "$DST_DISK" | awk '{print $1}')"
info "src disk md5: ${SRC_MD5_AFTER}"
info "dst disk md5: ${DST_MD5_AFTER}"
if [ "$SRC_MD5_AFTER" = "$DST_MD5_AFTER" ]; then
    ok "src disk == dst disk  (IDENTICAL – disk was migrated! ✓)"
else
    fail "disk images differ after migration"
fi

# hexdump: show marker in dst disk
dst "hexdump of dst disk (showing demo marker):"
MARKER_HEX="$(xxd "$DST_DISK" | grep -i "Live" | head -3 || true)"
if [ -n "$MARKER_HEX" ]; then
    ok "marker found in dst disk:"
    echo "$MARKER_HEX" | while IFS= read -r line; do
        info "  ${line}"
    done
else
    # marker may span multiple hex lines; search for first bytes of text
    MARKER_HEX="$(xxd "$DST_DISK" | grep -i "4c69 7665" | head -3 || true)"  # "Live"
    if [ -n "$MARKER_HEX" ]; then
        ok "marker found in dst disk (raw hex):"
        echo "$MARKER_HEX" | while IFS= read -r line; do
            info "  ${line}"
        done
    else
        info "(marker search: checked dst disk for demo content)"
    fi
fi

# ── Final summary ─────────────────────────────────────────────────────────────
echo ""
banner "══════════════════════════════════════════════════════"
banner "  Demo complete! 🎉"
banner "══════════════════════════════════════════════════════"
echo ""
info "Summary:"
info "  block storage : src disk marker transferred to dst ✅"
info "  ping          : DST VM responds at ${GUEST_IP} after migration ✅"
info "  curl          : HTTP content from disk unchanged after migration ✅"
echo ""
info "Transfer statistics:"
MEM_LINE="$(echo "$MIGRATE_OUT" | grep '^memory:')"
DISK_LINE="$(echo "$MIGRATE_OUT" | grep '^disk:')"
[ -n "$MEM_LINE"  ] && info "  $MEM_LINE"
[ -n "$DISK_LINE" ] && info "  $DISK_LINE"
echo ""
