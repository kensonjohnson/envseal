#!/usr/bin/env bash
set -euo pipefail

version=${1:?usage: release-build.sh v1.YYYYMMDD.N}
if [[ ! $version =~ ^v1\.[0-9]{8}\.[1-9][0-9]*$ ]]; then
  echo "release version must match v1.YYYYMMDD.N" >&2
  exit 2
fi

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
rm -rf build/release dist
mkdir -p build/release dist

for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  goos=${target%/*}
  goarch=${target#*/}
  name="envseal_${version}_${goos}_${goarch}"
  staged="build/release/${name}"
  binary=envseal
  if [[ $goos == windows ]]; then
    binary=envseal.exe
  fi

  mkdir -p "$staged"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOTOOLCHAIN=go1.24.0 \
    go build -trimpath -ldflags="-s -w -X main.version=${version}" -o "$staged/$binary" .

  if [[ $goos == windows ]]; then
    (cd "$staged" && zip -q "../../../dist/${name}.zip" "$binary")
  else
    (cd "$staged" && tar -czf "../../../dist/${name}.tar.gz" "$binary")
  fi
done

(cd dist && shasum -a 256 ./*.tar.gz ./*.zip > SHA256SUMS)
