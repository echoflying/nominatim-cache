CREATE TABLE IF NOT EXISTS cache_entries (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  cache_key VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  lat DOUBLE NOT NULL,
  lon DOUBLE NOT NULL,
  display_name TEXT,
  address LONGTEXT,
  place_type VARCHAR(64),
  source VARCHAR(255),
  nominatim_response LONGTEXT NOT NULL,
  first_cached_at BIGINT NOT NULL,
  last_accessed_at BIGINT NOT NULL,
  access_count BIGINT DEFAULT 1,
  UNIQUE KEY uq_cache_key (cache_key),
  KEY idx_first_cached (first_cached_at, id),
  KEY idx_last_accessed (last_accessed_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS stats (
  id INT NOT NULL PRIMARY KEY,
  total_requests BIGINT NOT NULL DEFAULT 0,
  cache_hits BIGINT NOT NULL DEFAULT 0,
  cache_misses BIGINT NOT NULL DEFAULT 0,
  last_updated_at BIGINT NOT NULL
) ENGINE=InnoDB;

INSERT IGNORE INTO stats (id, last_updated_at) VALUES (1, 0);

CREATE TABLE IF NOT EXISTS access_logs (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  cache_key VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin,
  lat DOUBLE,
  lon DOUBLE,
  cached TINYINT NOT NULL DEFAULT 0,
  accessed_at BIGINT NOT NULL,
  KEY idx_accessed_at (accessed_at, id),
  KEY idx_cache_key_logs (cache_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sqlite_imports (
  checksum CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  imported_at BIGINT NOT NULL,
  receipt LONGTEXT NOT NULL
) ENGINE=InnoDB;
