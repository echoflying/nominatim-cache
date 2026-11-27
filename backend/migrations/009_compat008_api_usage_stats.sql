CREATE TABLE IF NOT EXISTS api_request_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  event_day DATE NOT NULL,
  occurred_at BIGINT NOT NULL,
  caller_id VARBINARY(64) NOT NULL DEFAULT '',
  operation VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  client_platform VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'unknown',
  client_version VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  http_status SMALLINT UNSIGNED NOT NULL,
  duration_ms INT UNSIGNED NOT NULL,
  point_count INT UNSIGNED NOT NULL DEFAULT 0,
  cache_hits INT UNSIGNED NOT NULL DEFAULT 0,
  upstream_points INT UNSIGNED NOT NULL DEFAULT 0,
  KEY idx_api_event_day (event_day,id),
  KEY idx_api_event_caller (caller_id,event_day,operation)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS api_request_daily_rollups (
  event_day DATE NOT NULL,
  caller_id VARBINARY(64) NOT NULL DEFAULT '',
  operation VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  client_platform VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'unknown',
  client_version VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  status_class TINYINT UNSIGNED NOT NULL,
  requests BIGINT UNSIGNED NOT NULL,
  rate_limited BIGINT UNSIGNED NOT NULL,
  points BIGINT UNSIGNED NOT NULL,
  cache_hits BIGINT UNSIGNED NOT NULL,
  upstream_points BIGINT UNSIGNED NOT NULL,
  duration_sum_ms BIGINT UNSIGNED NOT NULL,
  duration_max_ms INT UNSIGNED NOT NULL,
  PRIMARY KEY (event_day,caller_id,operation,client_platform,client_version,status_class)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
