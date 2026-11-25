#!/usr/bin/env python3
"""Create a verified, private archive from the running Node SQLite database."""

import argparse
import hashlib
import json
import os
from contextlib import closing
from datetime import datetime, timezone
from pathlib import Path
import sqlite3
import tarfile
import tempfile


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def backup(source, output):
    source = source.resolve(strict=True)
    if not source.is_file():
        raise ValueError("source must be the old service's SQLite database file")
    output = output.absolute()
    if os.path.lexists(output):
        raise FileExistsError("output already exists")

    with tempfile.TemporaryDirectory(prefix=".nc-snapshot-", dir=output.parent) as temp:
        stage = Path(temp)
        snapshot = stage / "cache.db"
        # The SQLite backup API includes committed WAL pages in the snapshot.
        # mode=ro also makes a mistyped source path fail instead of creating a DB.
        with closing(sqlite3.connect(source.as_uri() + "?mode=ro", uri=True)) as old:
            with closing(sqlite3.connect(snapshot)) as new:
                old.backup(new, pages=256, sleep=0.1)
                new.execute("PRAGMA journal_mode=DELETE")
                checked = new.execute("PRAGMA integrity_check").fetchall()
                if checked != [("ok",)]:
                    raise ValueError("snapshot failed SQLite integrity_check")
                counts = {
                    table: new.execute("SELECT COUNT(*) FROM " + table).fetchone()[0]
                    for table in ("cache_entries", "access_logs", "stats")
                }
                if counts["stats"] != 1:
                    raise ValueError("snapshot must contain the legacy stats row")

        manifest = {
            "created_at_utc": datetime.now(timezone.utc).isoformat(),
            "sqlite_version": sqlite3.sqlite_version,
            "size": snapshot.stat().st_size,
            "sha256": sha256(snapshot),
            "counts": counts,
        }
        (stage / "manifest.json").write_text(
            json.dumps(manifest, indent=2) + "\n", encoding="utf-8"
        )
        (stage / "SHA256SUMS").write_text(
            manifest["sha256"] + "  cache.db\n", encoding="ascii"
        )
        archive = stage / "snapshot.tar.gz"
        with archive.open("xb") as stream:
            archive.chmod(0o600)
            with tarfile.open(fileobj=stream, mode="w:gz") as bundle:
                for filename in ("cache.db", "manifest.json", "SHA256SUMS"):
                    bundle.add(stage / filename, arcname=filename)
        # Publish only the complete archive, and never clobber a prior backup.
        os.link(archive, output)
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        manifest = backup(args.source, args.output)
    except (OSError, sqlite3.Error, ValueError) as exc:
        parser.exit(1, f"Backup failed: {exc}\n")
    print(json.dumps(manifest, ensure_ascii=False))


if __name__ == "__main__":
    main()
