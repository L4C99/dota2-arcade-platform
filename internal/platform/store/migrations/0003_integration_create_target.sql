ALTER TABLE node_jobs
    ADD COLUMN template_binding_key text CHECK (template_binding_key IS NULL OR length(template_binding_key) BETWEEN 1 AND 128),
    ADD COLUMN requested_port integer NOT NULL DEFAULT 0 CHECK (requested_port BETWEEN 0 AND 65535);

ALTER TABLE node_jobs ADD CONSTRAINT integration_create_target
    CHECK (kind <> 'stop' OR template_binding_key IS NULL);
