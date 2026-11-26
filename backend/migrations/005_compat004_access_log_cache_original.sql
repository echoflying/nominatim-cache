ALTER TABLE cache_entries ADD COLUMN IF NOT EXISTS upstream_archive_id BIGINT;

ALTER TABLE access_log_responses
  ADD COLUMN IF NOT EXISTS cache_original_archive_id BIGINT,
  ADD COLUMN IF NOT EXISTS cache_original_source VARCHAR(255),
  ADD COLUMN IF NOT EXISTS cache_original_body LONGBLOB;
