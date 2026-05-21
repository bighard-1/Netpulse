#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"
export JAVA_HOME="/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
/opt/homebrew/bin/gradle assembleDebug assembleRelease
mkdir -p "$ROOT/../../package/mobile"
cp "$ROOT/app/build/outputs/apk/debug/app-debug.apk" "$ROOT/../../package/mobile/NetPulse-Android-arm-debug-signed.apk"
cp "$ROOT/app/build/outputs/apk/release/app-release-unsigned.apk" "$ROOT/../../package/mobile/NetPulse-Android-arm-release-unsigned.apk"
cp "$ROOT/app/build/outputs/apk/debug/app-debug.apk" "$ROOT/../../package/mobile/NetPulse-Android-amd64-debug-signed.apk"
cp "$ROOT/app/build/outputs/apk/release/app-release-unsigned.apk" "$ROOT/../../package/mobile/NetPulse-Android-amd64-release-unsigned.apk"
echo "Android ARM debug signed APK: $ROOT/../../package/mobile/NetPulse-Android-arm-debug-signed.apk"
echo "Android ARM release APK: $ROOT/../../package/mobile/NetPulse-Android-arm-release-unsigned.apk"
echo "Android AMD64 debug signed APK: $ROOT/../../package/mobile/NetPulse-Android-amd64-debug-signed.apk"
echo "Android AMD64 release APK: $ROOT/../../package/mobile/NetPulse-Android-amd64-release-unsigned.apk"
