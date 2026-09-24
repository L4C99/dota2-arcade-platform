CREATE TABLE users (
    id uuid PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CHECK (expires_at > created_at)
);
CREATE INDEX user_sessions_user_id_idx ON user_sessions(user_id);

CREATE TABLE admin_users (
    id uuid PRIMARY KEY,
    username text NOT NULL UNIQUE CHECK (length(username) BETWEEN 1 AND 128),
    password_hash text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    credential_version bigint NOT NULL DEFAULT 1 CHECK (credential_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admin_sessions (
    id uuid PRIMARY KEY,
    admin_user_id uuid NOT NULL REFERENCES admin_users(id),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    credential_version bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CHECK (expires_at > created_at)
);
CREATE INDEX admin_sessions_user_id_idx ON admin_sessions(admin_user_id);

CREATE TABLE nodes (
    id uuid PRIMARY KEY,
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE node_jobs (
    id uuid PRIMARY KEY,
    node_id uuid NOT NULL REFERENCES nodes(id),
    kind text NOT NULL CHECK (kind IN ('create', 'stop')),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'claimed', 'accepted', 'unknown', 'succeeded', 'rejected_no_effect', 'failed_with_effect')),
    integration_only boolean NOT NULL DEFAULT false,
    instance_id text,
    operation_id text,
    error_code text,
    error_stage text,
    created_at timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX node_jobs_node_state_idx ON node_jobs(node_id, state, created_at);

CREATE TABLE node_job_executions (
    node_job_id uuid PRIMARY KEY REFERENCES node_jobs(id),
    core_idempotency_key text NOT NULL UNIQUE CHECK (core_idempotency_key ~ '^[A-Za-z0-9._-]{1,128}$'),
    resolved_template_path text NOT NULL CHECK (length(resolved_template_path) > 0),
    requested_port integer NOT NULL CHECK (requested_port BETWEEN 0 AND 65535),
    request_fingerprint bytea NOT NULL CHECK (octet_length(request_fingerprint) = 32),
    prepared_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION reject_node_job_execution_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'frozen NodeJob execution cannot be changed or deleted';
END;
$$;
CREATE TRIGGER node_job_execution_immutable BEFORE UPDATE OR DELETE ON node_job_executions
    FOR EACH ROW EXECUTE FUNCTION reject_node_job_execution_change();
