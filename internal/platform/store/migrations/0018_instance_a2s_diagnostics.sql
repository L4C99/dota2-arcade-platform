-- Amendment 001 finalization: current, administrator-only per-instance A2S facts.
-- The legacy node_entry_capabilities.a2s_query_ok column is deprecated and derived.
CREATE TABLE node_instance_a2s_diagnostics (
    node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    instance_id text NOT NULL,
    local_port integer NOT NULL CHECK (local_port BETWEEN 1 AND 65535),
    status text NOT NULL CHECK (status IN ('ok', 'failed')),
    checked_at timestamptz NOT NULL,
    reported_at timestamptz NOT NULL,
    PRIMARY KEY (node_id, instance_id),
    UNIQUE (node_id, local_port)
);
