-- The heartbeat references one complete inventory scan. A stale or failed
-- scan cannot be promoted by a later heartbeat without the same scan ID.
ALTER TABLE node_reports
    ADD COLUMN inventory_scan_id text,
    ADD COLUMN inventory_state text CHECK (inventory_state IN ('confirmed','unknown')),
    ADD CONSTRAINT node_report_inventory_pair CHECK
        ((inventory_scan_id IS NULL) = (inventory_state IS NULL));

-- A v1.0.2 create Job carries its own immutable expected machine identity.
-- Backfill I1 ValidationRuns so a 20→21 upgrade retains open durable work.
ALTER TABLE node_jobs
    ADD COLUMN expected_template_fingerprint_sha256 text,
    ADD COLUMN template_binding_generation bigint,
    ADD COLUMN expected_workshop_id text,
    ADD COLUMN expected_content_version_id text,
    ADD COLUMN expected_vpk_sha256 text;

UPDATE node_jobs j SET
    expected_template_fingerprint_sha256=v.template_fingerprint_sha256,
    template_binding_generation=v.template_binding_generation,
    expected_workshop_id=g.workshop_id,
    expected_content_version_id=v.content_version_id,
    expected_vpk_sha256=v.content_sha256
FROM allocations a JOIN validation_runs v ON v.id=a.validation_run_id
    JOIN arcade_games g ON g.id=a.arcade_game_id
WHERE j.allocation_id=a.id AND j.kind='create' AND j.required_capability='content_validation_v102';

ALTER TABLE node_jobs ADD CONSTRAINT v102_create_expected_identity CHECK
    (kind<>'create' OR required_capability<>'content_validation_v102' OR
     (COALESCE(expected_template_fingerprint_sha256 ~ '^[0-9a-f]{64}$',false) AND
      COALESCE(template_binding_generation>0,false) AND COALESCE(length(expected_workshop_id)>0,false) AND
      COALESCE(length(expected_content_version_id)>0,false) AND COALESCE(expected_vpk_sha256 ~ '^[0-9a-f]{64}$',false)));

CREATE FUNCTION enforce_job_expected_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.expected_template_fingerprint_sha256,NEW.template_binding_generation,
        NEW.expected_workshop_id,NEW.expected_content_version_id,NEW.expected_vpk_sha256)
       IS DISTINCT FROM
       (OLD.expected_template_fingerprint_sha256,OLD.template_binding_generation,
        OLD.expected_workshop_id,OLD.expected_content_version_id,OLD.expected_vpk_sha256) THEN
        RAISE EXCEPTION 'NodeJob expected machine identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER node_job_expected_identity_guard BEFORE UPDATE ON node_jobs
    FOR EACH ROW EXECUTE FUNCTION enforce_job_expected_identity();
