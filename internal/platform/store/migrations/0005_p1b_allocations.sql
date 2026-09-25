ALTER TABLE nodes
    ADD COLUMN accepting_new_requests boolean NOT NULL DEFAULT true,
    ADD COLUMN draining boolean NOT NULL DEFAULT false,
    ADD COLUMN desired_max_instances integer NOT NULL DEFAULT 0 CHECK (desired_max_instances >= 0);

ALTER TABLE server_requests ADD CONSTRAINT server_request_id_game_unique UNIQUE(id,arcade_game_id);
ALTER TABLE server_requests DROP CONSTRAINT server_requests_state_check;
ALTER TABLE server_requests ADD CONSTRAINT server_requests_state_check CHECK (state IN
    ('waiting','allocating','creating','running','stopping','ended','cancelled','unavailable','failed_unreclaimed','quarantined'));
DROP INDEX server_requests_one_blocking_user;
DROP INDEX server_requests_one_blocking_party;
CREATE UNIQUE INDEX server_requests_one_blocking_user ON server_requests(owner_user_id)
    WHERE owner_user_id IS NOT NULL AND state IN ('waiting','allocating','creating','running','stopping','failed_unreclaimed','quarantined');
CREATE UNIQUE INDEX server_requests_one_blocking_party ON server_requests(owner_party_id)
    WHERE owner_party_id IS NOT NULL AND state IN ('waiting','allocating','creating','running','stopping','failed_unreclaimed','quarantined');

CREATE TABLE node_content_bindings (
    node_id uuid NOT NULL REFERENCES nodes(id),
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    reported_content_version_id text,
    reported_state text NOT NULL DEFAULT 'unknown' CHECK (reported_state IN ('unknown','confirmed')),
    reported_at timestamptz,
    accepting_new_allocations boolean NOT NULL DEFAULT false,
    PRIMARY KEY (node_id,arcade_game_id),
    FOREIGN KEY (arcade_game_id,reported_content_version_id)
        REFERENCES content_versions(arcade_game_id,id),
    CHECK (reported_state <> 'confirmed' OR reported_content_version_id IS NOT NULL)
);

CREATE TABLE allocations (
    id uuid PRIMARY KEY,
    server_request_id uuid NOT NULL REFERENCES server_requests(id),
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    attempt_sequence integer NOT NULL CHECK (attempt_sequence > 0),
    node_id uuid NOT NULL REFERENCES nodes(id),
    content_version_id text NOT NULL,
    template_revision_id text NOT NULL,
    state text NOT NULL DEFAULT 'reserved' CHECK (state IN
        ('reserved','create_unknown','creating','running','stopping','failed_unreclaimed','quarantined','reclaimed','released_no_effect')),
    assigned_at timestamptz NOT NULL,
    create_started_at timestamptz,
    ready_at timestamptz,
    join_info_available_at timestamptz,
    reclaimed_at timestamptz,
    error_code text,
    UNIQUE (server_request_id,attempt_sequence),
    FOREIGN KEY (server_request_id,arcade_game_id) REFERENCES server_requests(id,arcade_game_id),
    FOREIGN KEY (arcade_game_id,content_version_id) REFERENCES content_versions(arcade_game_id,id),
    FOREIGN KEY (arcade_game_id,template_revision_id) REFERENCES template_revisions(arcade_game_id,id)
);
CREATE UNIQUE INDEX allocations_one_active_attempt ON allocations(server_request_id)
    WHERE state NOT IN ('reclaimed','released_no_effect');
CREATE INDEX allocations_occupied_node ON allocations(node_id,state);

CREATE FUNCTION reject_allocation_identity_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.server_request_id IS DISTINCT FROM OLD.server_request_id
        OR NEW.arcade_game_id IS DISTINCT FROM OLD.arcade_game_id
        OR NEW.attempt_sequence IS DISTINCT FROM OLD.attempt_sequence
        OR NEW.node_id IS DISTINCT FROM OLD.node_id
        OR NEW.content_version_id IS DISTINCT FROM OLD.content_version_id
        OR NEW.template_revision_id IS DISTINCT FROM OLD.template_revision_id THEN
        RAISE EXCEPTION 'Allocation attempt identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER allocation_identity_immutable BEFORE UPDATE ON allocations
    FOR EACH ROW EXECUTE FUNCTION reject_allocation_identity_change();
CREATE FUNCTION reject_allocation_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Allocation history cannot be deleted';
END;
$$;
CREATE TRIGGER allocation_no_delete BEFORE DELETE ON allocations
    FOR EACH ROW EXECUTE FUNCTION reject_allocation_delete();

ALTER TABLE node_jobs ADD COLUMN allocation_id uuid REFERENCES allocations(id);
ALTER TABLE node_jobs ADD CONSTRAINT node_job_business_owner
    CHECK (integration_only = (allocation_id IS NULL));
CREATE UNIQUE INDEX node_jobs_one_create_per_allocation ON node_jobs(allocation_id)
    WHERE kind='create' AND allocation_id IS NOT NULL;
CREATE UNIQUE INDEX node_jobs_one_open_stop_per_allocation ON node_jobs(allocation_id)
    WHERE kind='stop' AND allocation_id IS NOT NULL
      AND state IN ('pending','claimed','accepted','unknown');
