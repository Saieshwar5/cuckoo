#!/usr/bin/env bash
# The Android emulator on this laptop, without Android Studio and without
# sudo. Everything lands under your home directory.
#
#   scripts/emulator.sh install   # JDK, SDK tools, one system image, one device (about 3 GB, once)
#   scripts/emulator.sh start     # boot the device and wait until Android is up
#   scripts/emulator.sh stop
#   scripts/emulator.sh env       # print the exports a shell needs to use adb and emulator
#
# HEADLESS=1 start boots without a window, for automated checks.
set -euo pipefail

JDK_DIR="$HOME/.local/jdk17"
SDK="${ANDROID_HOME:-$HOME/Android/Sdk}"
AVD="cuckoo"
API="35"
IMAGE="system-images;android-$API;google_apis;x86_64"
LOG="$HOME/.cache/cuckoo-emulator.log"

JDK_URL="https://api.adoptium.net/v3/binary/latest/17/ga/linux/x64/jdk/hotspot/normal/eclipse?project=jdk"
CMDLINE_CANDIDATES=(
  "https://dl.google.com/android/repository/commandlinetools-linux-13114758_latest.zip"
  "https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip"
)

# Newer SDK tools keep devices under ~/.config/.android and older ones under
# ~/.android; the emulator only looks where ANDROID_AVD_HOME says, so say it.
AVD_HOME="${ANDROID_AVD_HOME:-$HOME/.config/.android/avd}"
[ -d "$AVD_HOME" ] || [ ! -d "$HOME/.android/avd" ] || AVD_HOME="$HOME/.android/avd"

export JAVA_HOME="$JDK_DIR"
export ANDROID_HOME="$SDK"
export ANDROID_SDK_ROOT="$SDK"
export ANDROID_AVD_HOME="$AVD_HOME"
export PATH="$JDK_DIR/bin:$SDK/cmdline-tools/latest/bin:$SDK/platform-tools:$SDK/emulator:$PATH"

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
hdr() { printf '\n\033[1m%s\033[0m\n' "$*"; }

env_exports() {
  cat <<E
export JAVA_HOME="$JDK_DIR"
export ANDROID_HOME="$SDK"
export ANDROID_SDK_ROOT="$SDK"
export ANDROID_AVD_HOME="$AVD_HOME"
export PATH="$JDK_DIR/bin:$SDK/cmdline-tools/latest/bin:$SDK/platform-tools:$SDK/emulator:\$PATH"
E
}

install() {
  [ -e /dev/kvm ] && [ -w /dev/kvm ] || die "/dev/kvm is not usable; the emulator needs hardware virtualisation"
  mkdir -p "$JDK_DIR" "$SDK" "$(dirname "$LOG")"

  if [ ! -x "$JDK_DIR/bin/java" ]; then
    hdr "Downloading a Java runtime (needed by the SDK tools)"
    curl -fsSL "$JDK_URL" | tar xz --strip-components=1 -C "$JDK_DIR"
  fi
  "$JDK_DIR/bin/java" -version 2>&1 | head -1

  if [ ! -x "$SDK/cmdline-tools/latest/bin/sdkmanager" ]; then
    hdr "Downloading the Android SDK command-line tools"
    local tmp zip="" url
    tmp=$(mktemp -d)
    for url in "${CMDLINE_CANDIDATES[@]}"; do
      if curl -fsSL -o "$tmp/tools.zip" "$url"; then zip="$tmp/tools.zip"; break; fi
    done
    [ -n "$zip" ] || die "could not download the command-line tools; check https://developer.android.com/studio#command-line-tools-only"
    unzip -q "$zip" -d "$tmp"
    mkdir -p "$SDK/cmdline-tools/latest"
    cp -r "$tmp/cmdline-tools/." "$SDK/cmdline-tools/latest/"
    rm -rf "$tmp"
  fi

  hdr "Installing platform tools, the emulator and one Android $API image (the big download)"
  yes | sdkmanager --licenses >/dev/null 2>&1 || true
  sdkmanager --install "platform-tools" "emulator" "platforms;android-$API" "$IMAGE" | grep -v -E '^\[=|^$' || true

  hdr "Creating the device '$AVD'"
  mkdir -p "$AVD_HOME"
  echo no | avdmanager create avd -n "$AVD" -k "$IMAGE" -d pixel_7 --force >/dev/null
  # Newer tools keep devices under ~/.config/.android; older ones under ~/.android.
  local ini
  ini=$(avdmanager list avd 2>/dev/null | awk -v n="$AVD" '$1=="Name:" && $2==n {f=1} f && $1=="Path:" {print $2; exit}')/config.ini
  [ -f "$ini" ] || die "the device was created but its config was not found"
  # A modest phone: two gigabytes, hardware keyboard so typing on the laptop works.
  for kv in "hw.ramSize=2048" "hw.keyboard=yes" "hw.gpu.enabled=yes" "hw.gpu.mode=auto" "disk.dataPartition.size=4G"; do
    local k="${kv%%=*}"
    grep -q "^$k=" "$ini" && sed -i "s|^$k=.*|$kv|" "$ini" || echo "$kv" >> "$ini"
  done

  hdr "Ready"
  echo "Device '$AVD' created. Start it with: make emulator"
  echo "For adb in your own shell:  eval \"\$(scripts/emulator.sh env)\""
}

booted() {
  [ "$(adb -e shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]
}

start() {
  [ -x "$SDK/emulator/emulator" ] || die "the emulator is not installed; run: make emulator-install"
  if booted; then echo "the emulator is already running"; return; fi
  hdr "Booting the emulator (a minute or two the first time)"
  local flags=(-avd "$AVD" -no-boot-anim -no-audio -netdelay none -netspeed full)
  [ "${HEADLESS:-}" = 1 ] && flags+=(-no-window -gpu swiftshader_indirect)
  nohup "$SDK/emulator/emulator" "${flags[@]}" > "$LOG" 2>&1 &
  adb wait-for-device >/dev/null 2>&1 || true
  for _ in $(seq 1 240); do
    booted && { echo "Android is up"; return; }
    sleep 1
  done
  tail -5 "$LOG" >&2
  die "the emulator did not finish booting; log: $LOG"
}

stop() {
  adb -e emu kill >/dev/null 2>&1 && echo "emulator stopped" || echo "no emulator was running"
}

case "${1:-}" in
  install) install ;;
  start) start ;;
  stop) stop ;;
  env) env_exports ;;
  *) sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//' ;;
esac
