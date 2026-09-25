CREATE TABLE site_announcements (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    message text NOT NULL DEFAULT '' CHECK (length(message) <= 1000),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO site_announcements(singleton) VALUES(true);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    actor_admin_user_id uuid REFERENCES admin_users(id),
    actor_kind text NOT NULL CHECK (actor_kind IN ('admin','system','operator_cli')),
    action text NOT NULL CHECK (length(action) BETWEEN 1 AND 100),
    target_type text NOT NULL CHECK (length(target_type) BETWEEN 1 AND 100),
    target_id text NOT NULL CHECK (length(target_id) BETWEEN 1 AND 200),
    result text NOT NULL CHECK (result IN ('succeeded','rejected')),
    state_change jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((actor_kind='admin') = (actor_admin_user_id IS NOT NULL))
);
CREATE INDEX audit_events_recent ON audit_events(created_at DESC,id DESC);
CREATE FUNCTION reject_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'AuditEvent history is immutable';
END;
$$;
CREATE TRIGGER audit_events_immutable BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_audit_mutation();

CREATE TABLE node_reconcile_requests (
    node_id uuid PRIMARY KEY REFERENCES nodes(id),
    requested_generation bigint NOT NULL DEFAULT 0,
    completed_generation bigint NOT NULL DEFAULT 0,
    requested_at timestamptz,
    completed_at timestamptz,
    CHECK (requested_generation >= completed_generation AND completed_generation >= 0)
);

CREATE TABLE allocation_reconcile_facts (
    allocation_id uuid PRIMARY KEY REFERENCES allocations(id),
    instance_id text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('active','reclaimed','identity_unverified','uncertain')),
    lifecycle text NOT NULL,
    process text NOT NULL,
    cleanup text NOT NULL,
    port integer NOT NULL CHECK (port BETWEEN 0 AND 65535),
    reported_at timestamptz NOT NULL DEFAULT now()
);
