#!/bin/bash
set -uo pipefail

version=${1:?usage: verify-linux-package.sh VERSION [amd64|arm64]}
arch=${2:-amd64}
install_deb=${INSTALL_DEB:-0}

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

fail=0
check() {
    local label=$1
    shift
    if "$@" >/dev/null 2>&1; then
        echo "ok    $label"
    else
        echo "FAIL  $label"
        fail=1
    fi
}

tarball=dist/pixgo-${version}-linux-${arch}.tar.gz
deb=dist/pixgo_${version}_${arch}.deb

check "desktop file is valid" desktop-file-validate packaging/linux/pixgo.desktop
check "AppStream metadata is valid" appstreamcli validate --no-net packaging/linux/io.github.oops1.PixGo.metainfo.xml
check "tarball exists" test -f "$tarball"
check "deb exists" test -f "$deb"
check "deb package name" bash -c "[ \"\$(dpkg-deb -f '$deb' Package)\" = pixgo ]"
check "deb version" bash -c "[ \"\$(dpkg-deb -f '$deb' Version)\" = '$version' ]"
check "deb ships the binary" bash -c "dpkg-deb -c '$deb' | grep -q ' ./usr/bin/pixgo\$'"
check "deb ships the desktop file" bash -c "dpkg-deb -c '$deb' | grep -q ' ./usr/share/applications/pixgo.desktop\$'"
check "deb ships the license" bash -c "dpkg-deb -c '$deb' | grep -q ' ./usr/share/doc/pixgo/LICENSE\$'"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
export HOME=$work/home
export XDG_CONFIG_HOME=$HOME/.config
mkdir -p "$HOME"

tar -xzf "$tarball" -C "$work"
dir=$work/pixgo-${version}-linux-${arch}
prefix=$work/prefix

check "install.sh runs" "$dir/install.sh" --prefix "$prefix" --set-default
check "binary installed" test -x "$prefix/bin/pixgo"
check "installed desktop file is valid" desktop-file-validate "$prefix/share/applications/pixgo.desktop"
check "desktop Exec uses the install prefix" grep -q "^Exec=$prefix/bin/pixgo %F\$" "$prefix/share/applications/pixgo.desktop"
check "icon installed" test -f "$prefix/share/icons/hicolor/256x256/apps/pixgo.png"
check "license installed" test -f "$prefix/share/doc/pixgo/LICENSE"
check "mimeapps.list written" grep -q "^image/png=pixgo.desktop;\$" "$XDG_CONFIG_HOME/mimeapps.list"

if command -v xdg-mime >/dev/null 2>&1; then
    export XDG_DATA_HOME=$prefix/share
    export XDG_DATA_DIRS=$prefix/share:/usr/share
    for mime in image/png image/jpeg image/webp image/gif image/svg+xml; do
        check "xdg-mime default for $mime" bash -c "[ \"\$(xdg-mime query default $mime)\" = pixgo.desktop ]"
    done
fi

cp "$XDG_CONFIG_HOME/mimeapps.list" "$work/mimeapps.before"
check "-set-default is idempotent" "$prefix/bin/pixgo" -set-default
check "mimeapps.list is unchanged by a second run" cmp "$work/mimeapps.before" "$XDG_CONFIG_HOME/mimeapps.list"

if [ "$arch" = "amd64" ]; then
    check "binary reports its version" bash -c "[ \"\$('$prefix/bin/pixgo' -version)\" = 'PixGo $version' ]"
fi

check "uninstall.sh runs" "$dir/uninstall.sh" --prefix "$prefix"
check "binary removed" test ! -e "$prefix/bin/pixgo"
check "desktop file removed" test ! -e "$prefix/share/applications/pixgo.desktop"

if [ "$install_deb" = "1" ] && [ "$arch" = "amd64" ]; then
    check "deb installs" sudo dpkg -i "$deb"
    check "installed deb binary runs" bash -c "[ \"\$(/usr/bin/pixgo -version)\" = 'PixGo $version' ]"
    check "deb desktop file validates" desktop-file-validate /usr/share/applications/pixgo.desktop
    check "deb removes cleanly" sudo dpkg -r pixgo
fi

if [ "$fail" -ne 0 ]; then
    echo "package verification failed"
    exit 1
fi
echo "package verified"
