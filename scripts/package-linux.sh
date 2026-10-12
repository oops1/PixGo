#!/bin/bash
set -euo pipefail

version=${1:?usage: package-linux.sh VERSION [amd64|arm64]}
arch=${2:-amd64}
nfpm_version=v2.47.0

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

dist=dist/linux
name=pixgo-${version}-linux-${arch}
stage=$dist/$name

mkdir -p "$dist" dist
CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X github.com/oops1/PixGo/internal/buildinfo.Version=$version" \
    -o "$dist/pixgo-$arch" ./cmd/pixgo

rm -rf "$stage"
mkdir -p "$stage/bin" "$stage/share/applications" "$stage/share/doc/pixgo/third_party"
cp "$dist/pixgo-$arch" "$stage/bin/pixgo"
cp packaging/linux/pixgo.desktop "$stage/share/applications/"
cp packaging/linux/install.sh packaging/linux/uninstall.sh "$stage/"
cp LICENSE NOTICE "$stage/share/doc/pixgo/"
cp third_party/* "$stage/share/doc/pixgo/third_party/"
for size in 16x16 32x32 48x48 64x64 128x128 256x256; do
    mkdir -p "$stage/share/icons/hicolor/$size/apps"
    cp "packaging/icons/hicolor/$size/apps/pixgo.png" "$stage/share/icons/hicolor/$size/apps/"
done
chmod 0755 "$stage/bin/pixgo" "$stage/install.sh" "$stage/uninstall.sh"

tar --owner=0 --group=0 --numeric-owner -C "$dist" -czf "dist/$name.tar.gz" "$name"

mkdir -p "$dist/pkgroot"
cp "$dist/pixgo-$arch" "$dist/pkgroot/pixgo"

for format in deb rpm; do
    VERSION=$version ARCH=$arch GOWORK=off \
        go run "github.com/goreleaser/nfpm/v2/cmd/nfpm@$nfpm_version" \
        package -f packaging/linux/nfpm.yaml -p "$format" -t dist/
done

ls -la dist/*.tar.gz dist/*.deb dist/*.rpm
