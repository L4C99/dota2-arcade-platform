ALTER TABLE platform_settings
    ADD COLUMN quarantine_after_node_unreachable interval NOT NULL DEFAULT interval '15 minutes'
        CHECK (quarantine_after_node_unreachable > interval '0 seconds');

ALTER TABLE server_requests DROP CONSTRAINT server_requests_state_check;
ALTER TABLE server_requests ADD CONSTRAINT server_requests_state_check CHECK (state IN
    ('waiting','allocating','creating','running','stopping','ended','cancelled','unavailable',
     'failed_unreclaimed','quarantined','abandoned'));
ALTER TABLE server_requests ADD COLUMN abandoned_at timestamptz;
ALTER TABLE server_requests ADD CONSTRAINT server_request_abandoned_at_check
    CHECK ((state = 'abandoned') = (abandoned_at IS NOT NULL));

ALTER TABLE allocations ADD COLUMN quarantined_at timestamptz;
