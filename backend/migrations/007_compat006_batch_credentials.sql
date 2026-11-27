CREATE TABLE IF NOT EXISTS batch_credentials (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  caller_id VARBINARY(64) NOT NULL,
  label VARCHAR(100) NOT NULL,
  token_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  token_hint VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  created_at BIGINT NOT NULL,
  last_used_at BIGINT NULL,
  revoked_at BIGINT NULL,
  UNIQUE KEY uq_batch_credentials_token_hash (token_hash),
  KEY idx_batch_credentials_caller (caller_id, revoked_at, created_at)
) ENGINE=InnoDB;
