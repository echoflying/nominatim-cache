ALTER TABLE batch_credentials
  ADD COLUMN token_nonce VARBINARY(12) NULL,
  ADD COLUMN token_ciphertext VARBINARY(128) NULL,
  ADD COLUMN last_revealed_at BIGINT NULL;
