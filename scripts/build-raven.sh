#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
version="${1:-}"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$ ]]; then
  echo "usage: bash scripts/build-raven.sh v2.0.0[-rc.N]" >&2
  exit 2
fi
commit="$(git rev-parse HEAD)"
if [[ -n "$(git status --porcelain)" ]]; then
  echo "release requires a clean working tree" >&2
  exit 2
fi
if [[ "$commit" != "$(git rev-parse --verify "refs/tags/$version^{commit}")" ]]; then
  echo "the release tag must point to HEAD" >&2
  exit 2
fi

base="nominatim-cache-$version-linux-amd64"
mkdir -p dist
if [[ -e "dist/$base.tar.gz" || -e "dist/$base.tar.gz.sha256" ]]; then
  echo "release archive already exists" >&2
  exit 2
fi
stage="$(mktemp -d "$root/dist/.package.XXXXXXXX")"
trap 'rm -rf -- "$stage"' EXIT
pkg="$stage/$base"
mkdir -p "$pkg/bin" "$pkg/migrations" "$pkg/deploy" "$pkg/tools"
(
  cd backend
  export CGO_ENABLED=0 GOOS=linux GOARCH=amd64
  go build -mod=readonly -buildvcs=false -trimpath -ldflags "-s -w -X main.version=$version -X main.commit=$commit" -o "$pkg/bin/nominatim-cache" ./cmd/nominatim-cache
  go build -mod=readonly -buildvcs=false -trimpath -ldflags '-s -w' -o "$pkg/bin/nc-migrate" ./cmd/nc-migrate
)
cp -R frontend "$pkg/frontend"
cp backend/migrations/*.sql "$pkg/migrations/"
cp deploy/raven/* "$pkg/deploy/"
cp scripts/backup-legacy-sqlite.py "$pkg/tools/"
printf '{"version":"%s","commit":"%s","target":"linux/amd64","schema":"%s"}\n' \
  "$version" "$commit" "$(basename "$(find "$pkg/migrations" -name '*.sql' -print | LC_ALL=C sort | tail -1)" .sql)" > "$pkg/release.json"
(
  cd "$pkg"
  find bin frontend migrations deploy tools -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > SHA256SUMS
  sha256sum release.json >> SHA256SUMS
)
tar -C "$stage" -czf "$stage/$base.tar.gz" "$base"
(cd "$stage" && sha256sum "$base.tar.gz" > "$base.tar.gz.sha256")
mv "$stage/$base.tar.gz" "$stage/$base.tar.gz.sha256" "$root/dist/"
printf '%s\n' "dist/$base.tar.gz" "dist/$base.tar.gz.sha256"
