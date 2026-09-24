ALTER TABLE nodes
    ADD COLUMN os text CHECK (os IN ('windows','linux')),
    ADD COLUMN secret_hash bytea UNIQUE CHECK (secret_hash IS NULL OR octet_length(secret_hash)=32),
    ADD COLUMN enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN last_heartbeat timestamptz;

CREATE TABLE node_reports (
    node_id uuid PRIMARY KEY REFERENCES nodes(id),
    controller_version text NOT NULL,
    node_api_version integer NOT NULL,
    d2core_version text NOT NULL,
    d2core_commit text NOT NULL,
    d2core_protocol_version integer NOT NULL,
    compatibility_status text NOT NULL CHECK (compatibility_status IN ('compatible','incompatible')),
    hard_max_instances integer NOT NULL CHECK (hard_max_instances >= 0),
    network_facts jsonb NOT NULL CHECK (jsonb_typeof(network_facts)='object'),
    content_facts jsonb NOT NULL CHECK (jsonb_typeof(content_facts)='array'),
    reported_at timestamptz NOT NULL
);

CREATE TABLE node_job_reports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    node_job_id uuid NOT NULL REFERENCES node_jobs(id),
    state text NOT NULL,
    instance_id text,
    operation_id text,
    error_code text,
    error_stage text,
    reported_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX node_job_reports_job_idx ON node_job_reports(node_job_id, id);
