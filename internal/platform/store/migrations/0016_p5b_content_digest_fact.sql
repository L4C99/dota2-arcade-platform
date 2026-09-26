ALTER TABLE node_content_bindings
    ADD COLUMN reported_content_sha256 text CHECK (reported_content_sha256 IS NULL OR reported_content_sha256 ~ '^[0-9a-f]{64}$');
