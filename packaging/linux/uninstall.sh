#!/bin/sh
set -eu

prefix=

while [ $# -gt 0 ]; do
    case "$1" in
        --prefix) prefix=${2:?missing directory}; shift 2 ;;
        --prefix=*) prefix=${1#*=}; shift ;;
        -h|--help) echo "Usage: uninstall.sh [--prefix DIR]"; exit 0 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

if [ -z "$prefix" ]; then
    if [ "$(id -u)" -eq 0 ]; then
        prefix=/usr/local
    else
        prefix=$HOME/.local
    fi
fi

rm -f "$prefix/bin/pixgo" "$prefix/share/applications/pixgo.desktop"
for dir in "$prefix"/share/icons/hicolor/*/apps; do
    [ -d "$dir" ] && rm -f "$dir/pixgo.png"
done
rm -rf "$prefix/share/doc/pixgo"

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q "$prefix/share/applications" 2>/dev/null || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f "$prefix/share/icons/hicolor" 2>/dev/null || true
fi

echo "PixGo removed from $prefix"
echo "Settings and favorites stay in ~/.config/pixgo and ~/.local/state/pixgo."
