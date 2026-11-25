#!/usr/bin/env bash
set -euo pipefail

archive=
expected=
upstream=
apply=false
while (($#)); do
  case "$1" in
    --archive) archive="${2:?missing archive path}"; shift 2 ;;
    --sha256) expected="${2:?missing SHA-256}"; shift 2 ;;
    --upstream) upstream="${2:?missing upstream URL}"; shift 2 ;;
    --apply) apply=true; shift ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done

die() { printf '%s\n' "$*" >&2; exit 1; }
[[ "$archive" = /* && -f "$archive" ]] || die 'Provide the absolute release archive path.'
[[ "$expected" =~ ^[[:xdigit:]]{64}$ ]] || die 'Provide the published release SHA-256.'
[[ "$upstream" =~ ^https://[A-Za-z0-9.-]+(/[A-Za-z0-9_./-]+)?$ ]] || die 'Provide a plain HTTPS reverse endpoint.'
[[ "$(uname -sm)" == 'Linux x86_64' ]] || die 'Expected Linux x86_64.'
[[ "$(findmnt -n -o TARGET -T /mnt/datadisk0)" == /mnt/datadisk0 ]] || die 'Raven data disk is not mounted.'

vendor_source=/mnt/datadisk0/elsewhere/software/mariadb-11.4.13-linux-systemd-x86_64
app_root=/opt/nominatim-cache
data_root=/mnt/datadisk0/nominatim-cache
config_root=/etc/nominatim-cache
unit_root=/etc/systemd/system
socket=/run/nominatim-cache-db/mariadb.sock
db_user=nominatim-cache-db
app_user=nominatim-cache

for path in "$app_root" "$data_root" "$config_root" /run/nominatim-cache-db \
  "$unit_root/nominatim-cache.service" "$unit_root/nominatim-cache-db.service"; do
  [[ ! -e "$path" && ! -L "$path" ]] || die "Fresh install blocked by existing path: $path"
done
for account in "$db_user" "$app_user"; do
  if getent passwd "$account" >/dev/null || getent group "$account" >/dev/null; then
    die "Fresh install blocked by existing account or group: $account"
  fi
done
listeners="$(ss -H -ltn)"
if awk '$4 ~ /:(18197|18198)$/ {found=1} END {exit !found}' <<< "$listeners"; then
  die 'Port 18197 or 18198 is already in use.'
fi
[[ -x "$vendor_source/bin/mariadbd" && -x "$vendor_source/bin/mariadb" && -x "$vendor_source/scripts/mariadb-install-db" ]] ||
  die 'Verified MariaDB 11.4.13 installation was not found.'

umask 077
stage="$(mktemp -d /var/tmp/nc-fresh.XXXXXXXX)"
trap 'rm -rf -- "$stage"' EXIT
cp -- "$archive" "$stage/release.tar.gz"
printf '%s  %s\n' "${expected,,}" "$stage/release.tar.gz" | sha256sum -c -
mkdir "$stage/pkg"
tar --no-same-owner --no-same-permissions -xzf "$stage/release.tar.gz" -C "$stage/pkg"
mapfile -t package_dirs < <(find "$stage/pkg" -mindepth 1 -maxdepth 1 -type d)
((${#package_dirs[@]} == 1)) || die 'Expected one release directory.'
package="${package_dirs[0]}"
(cd "$package" && sha256sum -c --quiet SHA256SUMS)
metadata="$(python3 - "$package/release.json" <<'PY'
import json, re, sys
with open(sys.argv[1], encoding="utf-8") as stream:
    manifest = json.load(stream)
version, commit = manifest["version"], manifest["commit"]
if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-rc\.\d+)?", version):
    raise SystemExit("Invalid release version")
if not re.fullmatch(r"[0-9a-f]{40}", commit) or manifest["target"] != "linux/amd64":
    raise SystemExit("Invalid release commit or target")
print(version, commit)
PY
)"
read -r version commit <<< "$metadata"
printf 'Fresh install preflight passed for %s (%s)\n' "$version" "$commit"
if ! "$apply"; then exit 0; fi
((EUID == 0)) || die 'Run --apply as root.'

useradd --system --user-group --no-create-home --shell /sbin/nologin "$db_user"
useradd --system --user-group --no-create-home --shell /sbin/nologin "$app_user"
install -d -m 0755 "$app_root" "$app_root/releases" "$config_root" "$data_root" "$data_root/vendor"
install -d -o "$db_user" -g "$db_user" -m 0750 "$data_root/mariadb"
install -d -m 0700 "$data_root/backups"
vendor="$data_root/vendor/mariadb-11.4.13"
cp -a --reflink=auto "$vendor_source" "$vendor"

release="$app_root/releases/$version"
mv -- "$package" "$release"
find "$release" -type d -exec chmod 0755 {} +
find "$release" -type f -exec chmod 0644 {} +
chmod 0755 "$release/bin/nominatim-cache" "$release/bin/nc-migrate"
ln -s -- "$release" "$app_root/current"
install -m 0644 "$release/deploy/mariadb.cnf" "$config_root/mariadb.cnf"

"$vendor/scripts/mariadb-install-db" --defaults-file="$config_root/mariadb.cnf" \
  --user="$db_user" --auth-root-authentication-method=socket \
  --auth-root-socket-user=root --skip-test-db
install -m 0644 "$release/deploy/nominatim-cache-db.service" "$unit_root/nominatim-cache-db.service"
systemctl daemon-reload
systemctl start nominatim-cache-db.service

sql=("$vendor/bin/mariadb" --defaults-file="$config_root/mariadb.cnf" --user=root --batch --skip-column-names)
dbstate="$("${sql[@]}" --execute 'SELECT VERSION(),@@datadir,@@socket,@@skip_networking')"
read -r mariadb_version datadir socket_path skip_networking <<< "$dbstate"
[[ "$mariadb_version" == 11.4.13-MariaDB* && "${datadir%/}" == "$data_root/mariadb" &&
   "$socket_path" == "$socket" && "$skip_networking" == 1 ]] || die "Unexpected private DB instance: $dbstate"

db_password="$(openssl rand -hex 24)"
admin_password="$(openssl rand -hex 24)"
printf '%s\n' \
  'CREATE DATABASE nominatim_cache CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;' \
  "CREATE USER 'nc_app'@'localhost' IDENTIFIED BY '$db_password';" \
  "GRANT SELECT, INSERT, UPDATE, DELETE ON nominatim_cache.* TO 'nc_app'@'localhost';" |
  "${sql[@]}"
MIGRATE_DSN="root@unix($socket)/nominatim_cache" "$release/bin/nc-migrate" schema

while IFS= read -r line || [[ -n "$line" ]]; do
  case "$line" in
    MARIADB_DSN=*) printf 'MARIADB_DSN="nc_app:%s@unix(%s)/nominatim_cache"\n' "$db_password" "$socket" ;;
    ADMIN_USERNAME=*) printf 'ADMIN_USERNAME=ncadmin\n' ;;
    ADMIN_PASSWORD=*) printf 'ADMIN_PASSWORD=%s\n' "$admin_password" ;;
    NOMINATIM_PRIMARY=*) printf 'NOMINATIM_PRIMARY=%s\n' "$upstream" ;;
    *) printf '%s\n' "$line" ;;
  esac
done < "$release/deploy/backend.env.example" > "$config_root/backend.env"
chmod 0600 "$config_root/backend.env"
unset db_password admin_password
install -m 0644 "$release/deploy/nominatim-cache.service" "$unit_root/nominatim-cache.service"
systemctl daemon-reload
systemctl start nominatim-cache.service

ready=false
for _ in {1..50}; do
  if health="$(curl -fsS --max-time 3 http://127.0.0.1:18197/nominatim/health 2>/dev/null)"; then
    ready=true
    break
  fi
  sleep 0.2
done
"$ready" || die 'Nominatim Cache did not pass the loopback health check.'
python3 -c 'import json,sys; h=json.load(sys.stdin); assert h["database_ready"] and h["version"]==sys.argv[1] and h["commit"]==sys.argv[2]; print(json.dumps(h))' \
  "$version" "$commit" <<< "$health"
[[ "$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:18197/nominatim/admin/stats)" == 404 ]] ||
  die 'Public listener unexpectedly serves admin routes.'
[[ "$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:18198/admin/stats)" == 401 ]] ||
  die 'Private admin listener did not require credentials.'
counts="$("${sql[@]}" nominatim_cache --execute 'SELECT (SELECT COUNT(*) FROM cache_entries),(SELECT COUNT(*) FROM access_logs),total_requests FROM stats WHERE id=1')"
[[ "$counts" == $'0\t0\t0' ]] || die "Database was not empty after setup: $counts"
printf 'MariaDB %s: cached addresses=0, access logs=0, requests=0\n' "$mariadb_version"
