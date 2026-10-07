#!/bin/sh
# Sestaví spustitelné soubory pro Windows, macOS a Linux do složky bin/.
# Potřebuje nainstalované Go (https://go.dev/dl/). Výsledné soubory jsou samostatné,
# nic dalšího se k jejich spuštění neinstaluje.
set -e
cd "$(dirname "$0")"
mkdir -p bin
build() {
	GOOS=$1 GOARCH=$2 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "bin/$3" .
	echo "bin/$3"
}
build windows amd64 llm-windows-amd64.exe
build darwin arm64 llm-macos-apple-silicon
build darwin amd64 llm-macos-intel
build linux amd64 llm-linux-amd64
