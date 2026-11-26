CREATE TABLE IF NOT EXISTS access_log_responses (
  access_log_id BIGINT NOT NULL PRIMARY KEY,
  upstream_source VARCHAR(255),
  upstream_body LONGBLOB,
  response_status SMALLINT UNSIGNED NOT NULL,
  response_body LONGTEXT NOT NULL,
  CONSTRAINT fk_access_log_responses_log FOREIGN KEY (access_log_id) REFERENCES access_logs(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
