CREATE TABLE IF NOT EXISTS upstream_archive (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  accessed_at BIGINT NOT NULL,
  origin VARCHAR(16) NOT NULL,
  source VARCHAR(255) NOT NULL,
  lat DOUBLE,
  lon DOUBLE,
  upstream_status SMALLINT UNSIGNED,
  response_body LONGBLOB,
  incomplete BOOLEAN NOT NULL DEFAULT FALSE,
  error_message VARCHAR(255),
  prior_log_id BIGINT,
  UNIQUE KEY uq_upstream_archive_prior_log (prior_log_id),
  KEY idx_upstream_archive_time (accessed_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE access_log_responses ADD COLUMN IF NOT EXISTS upstream_archived BOOLEAN NOT NULL DEFAULT FALSE;

INSERT INTO upstream_archive (accessed_at,origin,source,lat,lon,response_body,prior_log_id)
SELECT l.accessed_at,'reverse',r.upstream_source,l.lat,l.lon,r.upstream_body,l.id
FROM access_log_responses r JOIN access_logs l ON l.id=r.access_log_id
WHERE NOT r.upstream_archived AND r.upstream_source IS NOT NULL AND r.upstream_body IS NOT NULL
ON DUPLICATE KEY UPDATE prior_log_id=VALUES(prior_log_id);

CREATE TABLE IF NOT EXISTS access_daily_rollups (
  day DATE NOT NULL PRIMARY KEY,
  total BIGINT NOT NULL,
  hits BIGINT NOT NULL
) ENGINE=InnoDB;
