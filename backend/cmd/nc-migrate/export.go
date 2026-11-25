package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/echoflying/nominatim-cache/backend/migrations"
)

// Keep these columns and indexes compatible with server/src/services/database.ts.
const legacySchema = `
CREATE TABLE cache_entries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cache_key TEXT UNIQUE NOT NULL,
	lat REAL NOT NULL,
	lon REAL NOT NULL,
	display_name TEXT,
	address TEXT,
	place_type TEXT,
	source TEXT,
	nominatim_response TEXT NOT NULL,
	first_cached_at INTEGER NOT NULL,
	last_accessed_at INTEGER NOT NULL,
	access_count INTEGER DEFAULT 1
);
CREATE TABLE stats (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	total_requests INTEGER DEFAULT 0,
	cache_hits INTEGER DEFAULT 0,
	cache_misses INTEGER DEFAULT 0,
	last_updated_at INTEGER NOT NULL
);
CREATE TABLE access_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cache_key TEXT,
	lat REAL,
	lon REAL,
	cached INTEGER DEFAULT 0,
	accessed_at INTEGER NOT NULL
);
INSERT INTO stats (id, last_updated_at) VALUES (1, 0);
CREATE INDEX idx_cache_key ON cache_entries(cache_key);
CREATE INDEX idx_lat_lon ON cache_entries(lat,lon);
CREATE INDEX idx_accessed_at ON access_logs(accessed_at DESC);
CREATE INDEX idx_cache_key_logs ON access_logs(cache_key);
`

func exportSQLite(ctx context.Context, db *sql.DB, output string) (receipt, error) {
	output, err := filepath.Abs(output)
	if err != nil {
		return receipt{}, err
	}
	if _, err := os.Lstat(output); err == nil {
		return receipt{}, errors.New("导出路径已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return receipt{}, err
	}
	staging, err := os.CreateTemp(filepath.Dir(output), ".nc-export-*.db")
	if err != nil {
		return receipt{}, err
	}
	tmp := staging.Name()
	defer os.Remove(tmp)
	if err := staging.Close(); err != nil {
		return receipt{}, err
	}

	result, err := writeLegacyDB(ctx, db, tmp)
	if err != nil {
		return receipt{}, err
	}
	verified, sum, err := checkLegacyDB(ctx, tmp)
	if err != nil {
		return receipt{}, err
	}
	if result.DataSHA256 != verified.DataSHA256 || result.Counts != verified.Counts {
		return receipt{}, errors.New("导出 SQLite 的行数或内容摘要与 MariaDB 不一致")
	}
	// Validate the staging file first. Linking will not replace a file created
	// at output by a concurrent run.
	if err := os.Link(tmp, output); err != nil {
		return receipt{}, err
	}
	result.SHA256 = sum
	return result, nil
}

func writeLegacyDB(ctx context.Context, db *sql.DB, tmp string) (receipt, error) {
	snapshot, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return receipt{}, err
	}
	defer snapshot.Rollback()
	if err := migrations.Verify(ctx, snapshot); err != nil {
		return receipt{}, err
	}
	sqlite, err := sql.Open("sqlite", sqliteURI(tmp, "mode=rw"))
	if err != nil {
		return receipt{}, err
	}
	defer sqlite.Close()
	dst, err := sqlite.BeginTx(ctx, nil)
	if err != nil {
		return receipt{}, err
	}
	defer dst.Rollback()
	if _, err := dst.ExecContext(ctx, legacySchema); err != nil {
		return receipt{}, err
	}
	r := reader{db: snapshot, addressColumn: "address"}
	result, err := r.copy(ctx, dst)
	if err != nil {
		return receipt{}, err
	}
	if err := snapshot.Commit(); err != nil {
		return receipt{}, err
	}
	return result, dst.Commit()
}

func checkLegacyDB(ctx context.Context, path string) (receipt, string, error) {
	db, sum, err := openSnapshot(ctx, path)
	if err != nil {
		return receipt{}, "", err
	}
	defer db.Close()
	r, err := legacyReader(ctx, db)
	if err != nil {
		return receipt{}, "", err
	}
	result, err := r.copy(ctx, nil)
	return result, sum, err
}
