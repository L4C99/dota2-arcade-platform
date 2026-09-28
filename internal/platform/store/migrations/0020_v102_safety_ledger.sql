-- v1.0.2 I1: forward-only safety ledger. Historical migrations are immutable.
CREATE TABLE validation_runs (
    id uuid PRIMARY KEY,
    started_by_admin_user_id uuid NOT NULL REFERENCES admin_users(id),
    node_id uuid NOT NULL REFERENCES nodes(id),
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    game_preset_id uuid NOT NULL,
    content_version_id text NOT NULL,
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    template_revision_id text NOT NULL,
    template_binding_key text NOT NULL CHECK (length(template_binding_key) BETWEEN 1 AND 128),
    template_fingerprint_sha256 text NOT NULL CHECK (template_fingerprint_sha256 ~ '^[0-9a-f]{64}$'),
    template_binding_generation bigint NOT NULL CHECK (template_binding_generation > 0),
    maintenance_epoch bigint NOT NULL CHECK (maintenance_epoch > 0),
    content_fact_revision bigint NOT NULL CHECK (content_fact_revision > 0),
    template_fact_revision bigint NOT NULL CHECK (template_fact_revision > 0),
    state text NOT NULL DEFAULT 'create_pending' CHECK (state IN
        ('create_pending','create_observing','create_unknown','ready_no_join','awaiting_human',
         'cleanup_required','stop_pending','reclaim_observing','stop_unknown','quarantined','passed','failed')),
    failure_code text,
    result_code text,
    human_result text NOT NULL DEFAULT 'pending' CHECK (human_result IN ('pending','pass','fail')),
    human_confirmed_by uuid REFERENCES admin_users(id),
    human_confirmed_at timestamptz,
    ready_at timestamptz,
    join_info_available_at timestamptz,
    passed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((human_result='pending' AND human_confirmed_by IS NULL AND human_confirmed_at IS NULL)
        OR (human_result<>'pending' AND human_confirmed_by IS NOT NULL AND human_confirmed_at IS NOT NULL)),
    CHECK ((state='passed') = (passed_at IS NOT NULL)),
    CHECK (state<>'passed' OR human_result='pass'),
    FOREIGN KEY (game_preset_id,arcade_game_id) REFERENCES game_presets(id,arcade_game_id),
    FOREIGN KEY (arcade_game_id,content_version_id) REFERENCES content_versions(arcade_game_id,id),
    FOREIGN KEY (arcade_game_id,template_revision_id) REFERENCES template_revisions(arcade_game_id,id)
);

CREATE FUNCTION reject_validation_run_identity_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.id,NEW.started_by_admin_user_id,NEW.node_id,NEW.arcade_game_id,NEW.game_preset_id,
        NEW.content_version_id,NEW.content_sha256,NEW.template_revision_id,NEW.template_binding_key,
        NEW.template_fingerprint_sha256,NEW.template_binding_generation,NEW.maintenance_epoch,
        NEW.content_fact_revision,NEW.template_fact_revision)
       IS DISTINCT FROM
       (OLD.id,OLD.started_by_admin_user_id,OLD.node_id,OLD.arcade_game_id,OLD.game_preset_id,
        OLD.content_version_id,OLD.content_sha256,OLD.template_revision_id,OLD.template_binding_key,
        OLD.template_fingerprint_sha256,OLD.template_binding_generation,OLD.maintenance_epoch,
        OLD.content_fact_revision,OLD.template_fact_revision) THEN
        RAISE EXCEPTION 'ValidationRun identity is immutable';
    END IF;
    IF OLD.state IN ('passed','failed') AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'terminal ValidationRun is immutable';
    END IF;
    IF OLD.human_result <> 'pending' AND NEW.human_result IS DISTINCT FROM OLD.human_result THEN
        RAISE EXCEPTION 'human result is immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER validation_run_identity_immutable BEFORE UPDATE ON validation_runs
    FOR EACH ROW EXECUTE FUNCTION reject_validation_run_identity_change();
CREATE TRIGGER validation_run_no_delete BEFORE DELETE ON validation_runs
    FOR EACH ROW EXECUTE FUNCTION reject_allocation_delete();

CREATE TABLE maintenance_begin_requests (
    request_id text PRIMARY KEY CHECK (length(request_id) BETWEEN 1 AND 128),
    node_id uuid NOT NULL REFERENCES nodes(id),
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    epoch bigint NOT NULL CHECK (epoch > 0),
    actor_admin_user_id uuid NOT NULL REFERENCES admin_users(id),
    reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER maintenance_begin_no_change BEFORE UPDATE OR DELETE ON maintenance_begin_requests
    FOR EACH ROW EXECUTE FUNCTION reject_allocation_delete();

ALTER TABLE allocations
    ALTER COLUMN server_request_id DROP NOT NULL,
    ADD COLUMN purpose text NOT NULL DEFAULT 'player' CHECK (purpose IN ('player','validation')),
    ADD COLUMN validation_run_id uuid REFERENCES validation_runs(id),
    ADD CONSTRAINT allocation_owner_xor CHECK
        ((purpose='player' AND server_request_id IS NOT NULL AND validation_run_id IS NULL)
         OR (purpose='validation' AND server_request_id IS NULL AND validation_run_id IS NOT NULL
             AND attempt_sequence=1)),
    ADD CONSTRAINT allocation_validation_run_unique UNIQUE(validation_run_id);

CREATE FUNCTION reject_allocation_identity_change_v102() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.purpose,NEW.server_request_id,NEW.validation_run_id,NEW.arcade_game_id,
        NEW.attempt_sequence,NEW.node_id,NEW.content_version_id,NEW.template_revision_id)
       IS DISTINCT FROM
       (OLD.purpose,OLD.server_request_id,OLD.validation_run_id,OLD.arcade_game_id,
        OLD.attempt_sequence,OLD.node_id,OLD.content_version_id,OLD.template_revision_id) THEN
        RAISE EXCEPTION 'Allocation attempt identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER allocation_identity_immutable ON allocations;
CREATE TRIGGER allocation_identity_immutable BEFORE UPDATE ON allocations
    FOR EACH ROW EXECUTE FUNCTION reject_allocation_identity_change_v102();
CREATE FUNCTION validate_validation_allocation_identity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE run_record validation_runs%ROWTYPE;
BEGIN
    IF NEW.purpose='validation' THEN
        SELECT * INTO run_record FROM validation_runs WHERE id=NEW.validation_run_id;
        IF NOT FOUND OR (NEW.node_id,NEW.arcade_game_id,NEW.content_version_id,NEW.template_revision_id)
           IS DISTINCT FROM
           (run_record.node_id,run_record.arcade_game_id,run_record.content_version_id,run_record.template_revision_id) THEN
            RAISE EXCEPTION 'Validation Allocation identity differs from Run';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER allocation_validation_identity BEFORE INSERT ON allocations
    FOR EACH ROW EXECUTE FUNCTION validate_validation_allocation_identity();

ALTER TABLE node_content_bindings
    ADD COLUMN maintenance_epoch bigint NOT NULL DEFAULT 0 CHECK (maintenance_epoch >= 0),
    ADD COLUMN content_fact_revision bigint NOT NULL DEFAULT 0 CHECK (content_fact_revision >= 0);
ALTER TABLE node_reports ADD COLUMN capabilities text[] NOT NULL DEFAULT '{}';
ALTER TABLE node_template_bindings
    ADD COLUMN expected_template_fingerprint_sha256 text
        CHECK (expected_template_fingerprint_sha256 IS NULL OR expected_template_fingerprint_sha256 ~ '^[0-9a-f]{64}$'),
    ADD COLUMN binding_generation bigint NOT NULL DEFAULT 1 CHECK (binding_generation > 0);
CREATE FUNCTION enforce_template_binding_generation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.binding_key,NEW.expected_template_fingerprint_sha256)
       IS DISTINCT FROM (OLD.binding_key,OLD.expected_template_fingerprint_sha256) THEN
        IF NEW.binding_generation<>OLD.binding_generation+1 THEN
            RAISE EXCEPTION 'template binding identity change must advance generation once';
        END IF;
    ELSIF NEW.binding_generation<>OLD.binding_generation THEN
        RAISE EXCEPTION 'unchanged template binding cannot advance generation';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER template_binding_generation_guard BEFORE UPDATE ON node_template_bindings
    FOR EACH ROW EXECUTE FUNCTION enforce_template_binding_generation();

CREATE TABLE node_template_facts (
    node_id uuid NOT NULL,
    template_revision_id text NOT NULL,
    binding_key text,
    manifest_algorithm text,
    reported_state text NOT NULL DEFAULT 'unknown' CHECK (reported_state IN ('unknown','confirmed')),
    reported_fingerprint_sha256 text CHECK (reported_fingerprint_sha256 IS NULL OR reported_fingerprint_sha256 ~ '^[0-9a-f]{64}$'),
    template_fact_revision bigint NOT NULL DEFAULT 0 CHECK (template_fact_revision >= 0),
    received_at timestamptz,
    PRIMARY KEY (node_id,template_revision_id),
    FOREIGN KEY (node_id,template_revision_id) REFERENCES node_template_bindings(node_id,template_revision_id),
    CHECK (reported_state<>'confirmed' OR
        (binding_key IS NOT NULL AND manifest_algorithm='template-manifest-sha256-v1'
         AND reported_fingerprint_sha256 IS NOT NULL))
);

CREATE TABLE node_inventory_snapshots (
    node_id uuid NOT NULL REFERENCES nodes(id),
    scan_id text NOT NULL CHECK (length(scan_id) BETWEEN 1 AND 128),
    complete boolean NOT NULL,
    state text NOT NULL CHECK (state IN ('unknown','confirmed')),
    error_code text,
    received_at timestamptz NOT NULL DEFAULT now(),
    instance_count integer NOT NULL CHECK (instance_count >= 0),
    unaccounted_count integer NOT NULL CHECK (unaccounted_count >= 0),
    unaccounted_instance_ids text[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (node_id,scan_id),
    CHECK (state='unknown' OR complete),
    CHECK (state<>'confirmed' OR error_code IS NULL)
);
CREATE TABLE node_inventory_instances (
    node_id uuid NOT NULL,
    scan_id text NOT NULL,
    instance_id text NOT NULL CHECK (length(instance_id) > 0),
    lifecycle text NOT NULL,
    process text NOT NULL,
    cleanup text NOT NULL,
    current_operation_id text,
    accounted_allocation_id uuid REFERENCES allocations(id),
    PRIMARY KEY (node_id,scan_id,instance_id),
    FOREIGN KEY (node_id,scan_id) REFERENCES node_inventory_snapshots(node_id,scan_id)
);
CREATE INDEX node_inventory_latest ON node_inventory_snapshots(node_id,received_at DESC);

ALTER TABLE node_jobs
    ADD COLUMN required_capability text NOT NULL DEFAULT 'legacy_v1'
        CHECK (required_capability IN ('legacy_v1','content_validation_v102'));
ALTER TABLE node_job_executions
    ADD COLUMN required_capability text NOT NULL DEFAULT 'legacy_v1'
        CHECK (required_capability IN ('legacy_v1','content_validation_v102')),
    ADD COLUMN template_manifest_algorithm text,
    ADD COLUMN template_fingerprint_sha256 text
        CHECK (template_fingerprint_sha256 IS NULL OR template_fingerprint_sha256 ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT frozen_template_manifest_pair CHECK
        ((template_manifest_algorithm IS NULL) = (template_fingerprint_sha256 IS NULL)),
    ADD CONSTRAINT new_frozen_execution_manifest CHECK
        (required_capability='legacy_v1' OR
         (template_manifest_algorithm='template-manifest-sha256-v1' AND template_fingerprint_sha256 IS NOT NULL));
CREATE FUNCTION enforce_node_job_capability() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE required text;
BEGIN
    IF TG_TABLE_NAME='node_jobs' THEN
        IF TG_OP='UPDATE' AND NEW.required_capability <> OLD.required_capability THEN
            RAISE EXCEPTION 'NodeJob capability is immutable';
        END IF;
        IF NEW.allocation_id IS NOT NULL THEN
            IF EXISTS(SELECT 1 FROM allocations WHERE id=NEW.allocation_id AND purpose='validation')
               AND NEW.required_capability<>'content_validation_v102' THEN
                RAISE EXCEPTION 'Validation Job requires v1.0.2 capability';
            END IF;
        END IF;
    ELSE
        SELECT required_capability INTO required FROM node_jobs WHERE id=NEW.node_job_id;
        IF NEW.required_capability <> required THEN
            RAISE EXCEPTION 'frozen execution capability must match NodeJob';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER node_job_capability_guard BEFORE INSERT OR UPDATE ON node_jobs
    FOR EACH ROW EXECUTE FUNCTION enforce_node_job_capability();
CREATE TRIGGER frozen_execution_capability_guard BEFORE INSERT ON node_job_executions
    FOR EACH ROW EXECUTE FUNCTION enforce_node_job_capability();

-- A committed Run must own exactly one reservation and its one create Job.
-- The check is deferred so the Store may insert the three rows in one tx.
CREATE FUNCTION require_complete_validation_reserve() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE allocation_count integer; create_count integer;
BEGIN
    SELECT count(*) INTO allocation_count FROM allocations WHERE validation_run_id=NEW.id;
    SELECT count(*) INTO create_count FROM node_jobs j JOIN allocations a ON a.id=j.allocation_id
        WHERE a.validation_run_id=NEW.id AND j.kind='create' AND NOT j.integration_only;
    IF allocation_count<>1 OR create_count<>1 THEN
        RAISE EXCEPTION 'ValidationRun requires one Allocation and one create Job in the same commit';
    END IF;
    RETURN NEW;
END;
$$;
CREATE CONSTRAINT TRIGGER validation_run_reserve_complete
    AFTER INSERT ON validation_runs DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION require_complete_validation_reserve();

ALTER TABLE game_presets
    ADD COLUMN validation_contract text NOT NULL DEFAULT 'v1_0_2'
        CHECK (validation_contract IN ('legacy_v1','v1_0_2'));
UPDATE game_presets SET validation_contract='legacy_v1';
CREATE FUNCTION reject_validation_contract_downgrade() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.validation_contract='v1_0_2' AND NEW.validation_contract<>'v1_0_2' THEN
        RAISE EXCEPTION 'validation contract cannot be downgraded';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER game_preset_contract_no_downgrade BEFORE UPDATE ON game_presets
    FOR EACH ROW EXECUTE FUNCTION reject_validation_contract_downgrade();

CREATE TABLE content_releases (
    id uuid PRIMARY KEY,
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    old_content_version_id text,
    new_content_version_id text NOT NULL,
    published_by uuid NOT NULL REFERENCES admin_users(id),
    published_at timestamptz NOT NULL DEFAULT now(),
    rollback_of_release_id uuid REFERENCES content_releases(id),
    UNIQUE(id,arcade_game_id),
    FOREIGN KEY (arcade_game_id,old_content_version_id) REFERENCES content_versions(arcade_game_id,id),
    FOREIGN KEY (arcade_game_id,new_content_version_id) REFERENCES content_versions(arcade_game_id,id)
);
CREATE TABLE content_release_presets (
    release_id uuid NOT NULL REFERENCES content_releases(id),
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    game_preset_id uuid NOT NULL,
    old_template_revision_id text NOT NULL,
    new_template_revision_id text NOT NULL,
    old_accepting_new_requests boolean NOT NULL,
    new_accepting_new_requests boolean NOT NULL,
    validation_run_id uuid REFERENCES validation_runs(id),
    validation_node_id uuid REFERENCES nodes(id),
    PRIMARY KEY (release_id,game_preset_id),
    FOREIGN KEY (release_id,arcade_game_id) REFERENCES content_releases(id,arcade_game_id),
    FOREIGN KEY (game_preset_id,arcade_game_id) REFERENCES game_presets(id,arcade_game_id),
    FOREIGN KEY (arcade_game_id,old_template_revision_id) REFERENCES template_revisions(arcade_game_id,id),
    FOREIGN KEY (arcade_game_id,new_template_revision_id) REFERENCES template_revisions(arcade_game_id,id)
);
CREATE TRIGGER content_release_no_change BEFORE UPDATE OR DELETE ON content_releases
    FOR EACH ROW EXECUTE FUNCTION reject_allocation_delete();
CREATE TRIGGER content_release_preset_no_change BEFORE UPDATE OR DELETE ON content_release_presets
    FOR EACH ROW EXECUTE FUNCTION reject_allocation_delete();
