CREATE TABLE node_entry_capabilities (
    node_id uuid PRIMARY KEY REFERENCES nodes(id),
    entry_config_revision text NOT NULL CHECK (entry_config_revision ~ '^[0-9a-f]{64}$'),
    a2s_enabled boolean NOT NULL DEFAULT false,
    a2s_query_ok boolean NOT NULL DEFAULT false,
    steam_entry_verified boolean NOT NULL DEFAULT false,
    steam_entry_enabled boolean NOT NULL DEFAULT false,
    steam_verified_at timestamptz,
    steam_verified_by uuid REFERENCES admin_users(id),
    steamchina_entry_verified boolean NOT NULL DEFAULT false,
    steamchina_entry_enabled boolean NOT NULL DEFAULT false,
    steamchina_verified_at timestamptz,
    steamchina_verified_by uuid REFERENCES admin_users(id),
    reported_at timestamptz NOT NULL,
    CHECK (NOT steam_entry_enabled OR steam_entry_verified),
    CHECK (NOT steamchina_entry_enabled OR steamchina_entry_verified)
);

ALTER TABLE allocations
    ADD COLUMN join_local_port integer CHECK (join_local_port BETWEEN 1 AND 65535),
    ADD COLUMN join_public_port integer CHECK (join_public_port BETWEEN 1 AND 65535),
    ADD COLUMN join_connect_host text,
    ADD COLUMN join_protocol_ip text,
    ADD COLUMN join_entry_config_revision text CHECK (join_entry_config_revision IS NULL OR join_entry_config_revision ~ '^[0-9a-f]{64}$'),
    ADD COLUMN join_info_error_code text,
    ADD CONSTRAINT join_info_requires_ready CHECK (
        join_info_available_at IS NULL OR
        (ready_at IS NOT NULL AND join_local_port IS NOT NULL AND join_public_port IS NOT NULL
         AND join_connect_host IS NOT NULL AND join_entry_config_revision IS NOT NULL
         AND join_info_error_code IS NULL)
    );
