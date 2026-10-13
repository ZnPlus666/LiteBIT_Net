#!/bin/sh
# Shared launcher: POSIX shell and standard system utilities only.
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ "$(uname -s)" != Linux ]; then
    printf '%s\n' '此版本仅适用于 Linux。 / This version requires Linux.'
    exit 1
fi
case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) printf '%s\n' '不支持当前处理器架构；仅支持 x64 和 ARM64。 / Unsupported processor architecture; x64 and ARM64 only.'; exit 1 ;;
esac
PROGRAM="$SCRIPT_DIR/bin/litebit_linux_link2user_$ARCH"
if [ ! -f "$PROGRAM" ] || [ -L "$PROGRAM" ]; then
    printf '%s\n' '未找到程序，请下载完整发行包。 / Program not found; download the complete release package.'
    exit 1
fi
if [ ! -x "$PROGRAM" ]; then
    if ! chmod u+x "$PROGRAM"; then
        printf '%s\n' '无法设置执行权限。 / Cannot set execution permission.'
        exit 1
    fi
fi
exec "$PROGRAM" "$@"
