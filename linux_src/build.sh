#!/bin/sh
# Developer build entry; end users use the bundled binaries, not this file.
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$SCRIPT_DIR"
export CGO_ENABLED=0 GOTOOLCHAIN=local GOAMD64=v1 GOARM64=v8.0
go test -count=1 ./...
for mode in link2user link2PC; do
    folder="../LiteBIT_Net_linux_$mode"
    mkdir -p "$folder/bin"
    for arch in amd64 arm64; do
        GOOS=linux GOARCH="$arch" go build -trimpath -buildvcs=false \
            -ldflags "-s -w -X main.mode=$mode -X main.buildVersion=native-v2-$mode" \
            -o "$folder/bin/litebit_linux_${mode}_$arch" .
    done
    (cd "$folder" && sha256sum bin/litebit_linux_* > SHA256SUMS)
done
