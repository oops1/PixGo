#!/bin/sh
set -eu

usage() {
    cat <<'EOF'
Usage: install.sh [--prefix DIR] [--set-default]

  --prefix DIR    install under DIR (default: ~/.local, or /usr/local for root)
  --set-default   make PixGo the default viewer for image files
EOF
}

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
prefix=
set_default=0

while [ $# -gt 0 ]; do
    case "$1" in
        --prefix) prefix=${2:?missing directory}; shift 2 ;;
        --prefix=*) prefix=${1#*=}; shift ;;
        --set-default) set_default=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

if [ -z "$prefix" ]; then
    if [ "$(id -u)" -eq 0 ]; then
        prefix=/usr/local
    else
        prefix=$HOME/.local
    fi
fi

bin=$prefix/bin/pixgo
apps=$prefix/share/applications
doc=$prefix/share/doc/pixgo

install -d "$prefix/bin" "$apps" "$doc"
install -m 0755 "$here/bin/pixgo" "$bin"

sed -e "s|^Exec=pixgo|Exec=$bin|" -e "s|^TryExec=pixgo|TryExec=$bin|" \
    "$here/share/applications/pixgo.desktop" > "$apps/pixgo.desktop"
chmod 0644 "$apps/pixgo.desktop"

for dir in "$here"/share/icons/hicolor/*/apps; do
    size=$(basename "$(dirname "$dir")")
    install -d "$prefix/share/icons/hicolor/$size/apps"
    install -m 0644 "$dir/pixgo.png" "$prefix/share/icons/hicolor/$size/apps/pixgo.png"
done

for f in LICENSE NOTICE; do
    [ -f "$here/share/doc/pixgo/$f" ] && install -m 0644 "$here/share/doc/pixgo/$f" "$doc/$f"
done
if [ -d "$here/share/doc/pixgo/third_party" ]; then
    install -d "$doc/third_party"
    for f in "$here"/share/doc/pixgo/third_party/*; do
        install -m 0644 "$f" "$doc/third_party/$(basename "$f")"
    done
fi

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q "$apps" 2>/dev/null || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f "$prefix/share/icons/hicolor" 2>/dev/null || true
fi

if [ "$set_default" -eq 1 ]; then
    PIXGO_DESKTOP_ID=pixgo.desktop "$bin" -set-default
    echo "PixGo is now the default image viewer."
fi

echo "PixGo installed to $prefix"
case ":$PATH:" in
    *":$prefix/bin:"*) ;;
    *) echo "Note: $prefix/bin is not in PATH; the menu entry uses the full path." ;;
esac
if [ "$set_default" -eq 0 ]; then
    echo "Run '$bin -set-default' to make PixGo the default viewer for images."
fi
