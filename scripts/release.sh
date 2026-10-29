#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
export CGO_ENABLED=0
go test ./...
go run github.com/egoist/mygo/cmd/mygo@v0.7.2 build -platform darwin/arm64
version=$(sed -n 's/.*"version": "\([^"]*\)".*/\1/p' mygo.json)
out=build/darwin-arm64
ditto -c -k --sequesterRsrc --keepParent "$out/PortGlass.app" "$out/portglass-$version-arm64.zip"
cd "$out"
shasum -a 256 ./*.dmg ./*.zip > SHA256SUMS.txt
codesign --verify --deep --strict PortGlass.app
