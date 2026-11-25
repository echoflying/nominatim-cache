CREATE TABLE IF NOT EXISTS batch_callers (
  caller VARBINARY(64) PRIMARY KEY
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS batch_jobs (
  id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  caller VARBINARY(64) NOT NULL,
  request_key VARBINARY(128) NOT NULL,
  input_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  created_at BIGINT NOT NULL,
  finished_at BIGINT NULL,
  UNIQUE KEY uq_batch_request (caller, request_key),
  KEY idx_batch_finished (finished_at),
  CONSTRAINT fk_batch_caller FOREIGN KEY (caller) REFERENCES batch_callers(caller)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS geocode_tasks (
  cache_key VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  lat DOUBLE NOT NULL,
  lon DOUBLE NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  next_at BIGINT NOT NULL,
  lease_until BIGINT NULL,
  lease_token CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
  last_error VARCHAR(160) NULL,
  KEY idx_geocode_next (next_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS batch_items (
  job_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  ordinal INT NOT NULL,
  lat DOUBLE NOT NULL,
  lon DOUBLE NOT NULL,
  cache_key VARCHAR(48) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  result LONGTEXT NULL,
  last_error VARCHAR(160) NULL,
  PRIMARY KEY (job_id, ordinal),
  KEY idx_batch_item_cache (cache_key, status, job_id),
  CONSTRAINT fk_batch_job FOREIGN KEY (job_id) REFERENCES batch_jobs(id) ON DELETE CASCADE
) ENGINE=InnoDB;
