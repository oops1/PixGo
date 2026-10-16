#!/bin/bash
set -uo pipefail

duration=${1:-10m}
batch=${BATCH:-11}
workers=${WORKERS:-2}

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
mkdir -p dist/fuzz

targets=(
    "internal/codec/apng FuzzDecode"
    "internal/codec/apng FuzzProbe"
    "internal/codec/apng FuzzSniff"
    "internal/codec/raw FuzzDecode"
    "internal/codec/raw FuzzProbe"
    "internal/codec/raw FuzzSniff"
    "internal/codec/bmp FuzzDecode"
    "internal/codec/png FuzzDecode"
    "internal/codec/svg FuzzDecode"
    "internal/codec/gif FuzzDecode"
    "internal/codec/ico FuzzDecode"
    "internal/codec/webp FuzzDecode"
    "internal/codec/webp FuzzProbe"
    "internal/codec/avif FuzzDecode"
    "internal/codec/avif FuzzProbe"
    "internal/codec/heic FuzzDecode"
    "internal/codec/heic FuzzProbe"
    "internal/codec/jxl FuzzDecode"
    "internal/codec/jxl FuzzProbe"
    "internal/codec/tiff FuzzDecode"
    "internal/codec/tiff FuzzTiledReader"
    "internal/isobmff FuzzOpen"
    "internal/meta/exif FuzzDecode"
    "internal/meta FuzzRead"
    "internal/codec/jpeg FuzzDecode"
    "internal/codec/jpeg FuzzTileReader"
    "internal/jpegdec FuzzDecode"
    "internal/jpegdec FuzzRegion"
    "internal/jpegxform FuzzTransform"
    "internal/imaging FuzzTileManager"
    "internal/render FuzzWindowRender"
    "internal/platform FuzzProcInfoParsers"
)

if [ -n "${FILTER:-}" ]; then
    selected=()
    for t in "${targets[@]}"; do
        if [[ "$t" =~ $FILTER ]]; then
            selected+=("$t")
        fi
    done
    targets=("${selected[@]}")
fi

failed=0
run() {
    local pkg=$1 name=$2
    local log="dist/fuzz/$(echo "$pkg" | tr '/' '_')_$name.log"
    if go test -run=NONE -fuzz="^$name\$" -fuzztime="$duration" -parallel "$workers" "./$pkg" >"$log" 2>&1; then
        echo "ok    $pkg $name"
    else
        echo "FAIL  $pkg $name (see $log)"
        return 1
    fi
}

i=0
pids=()
for t in "${targets[@]}"; do
    set -- $t
    run "$1" "$2" &
    pids+=($!)
    i=$((i + 1))
    if [ $((i % batch)) -eq 0 ]; then
        for p in "${pids[@]}"; do wait "$p" || failed=1; done
        pids=()
    fi
done
for p in "${pids[@]}"; do wait "$p" || failed=1; done

if [ "$failed" -ne 0 ]; then
    echo "fuzzing found a failure"
    exit 1
fi
echo "fuzzing finished without failures"
