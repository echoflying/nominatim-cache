ALTER TABLE batch_jobs
  ADD COLUMN IF NOT EXISTS priority TINYINT UNSIGNED NOT NULL DEFAULT 2;

ALTER TABLE geocode_tasks
  ADD COLUMN IF NOT EXISTS priority TINYINT UNSIGNED NOT NULL DEFAULT 2,
  ADD COLUMN IF NOT EXISTS enqueued_at BIGINT NOT NULL DEFAULT 0;

UPDATE geocode_tasks SET enqueued_at=next_at WHERE enqueued_at=0;

CREATE TABLE IF NOT EXISTS batch_result_events (
  event_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  job_id CHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  ordinal INT NOT NULL,
  UNIQUE KEY uq_batch_result_event (job_id,ordinal),
  KEY idx_batch_result_cursor (job_id,event_id),
  CONSTRAINT fk_batch_result_event_job FOREIGN KEY (job_id) REFERENCES batch_jobs(id) ON DELETE CASCADE,
  CONSTRAINT fk_batch_result_event_item FOREIGN KEY (job_id,ordinal) REFERENCES batch_items(job_id,ordinal) ON DELETE CASCADE
) ENGINE=InnoDB;

INSERT IGNORE INTO batch_result_events (job_id,ordinal)
SELECT job_id,ordinal
FROM batch_items
WHERE status <> 'pending'
ORDER BY job_id,ordinal;
