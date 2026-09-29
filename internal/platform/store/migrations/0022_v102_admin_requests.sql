-- Durable results for I3 resource-creating Admin actions. Rows never change.
CREATE TABLE admin_i3_requests (
    action text NOT NULL CHECK (action IN ('validation.start','release.publish')),
    request_id text NOT NULL CHECK (length(request_id) BETWEEN 1 AND 128),
    payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    result_id uuid NOT NULL,
    actor_admin_user_id uuid NOT NULL REFERENCES admin_users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (action,request_id)
);
CREATE TRIGGER admin_i3_request_no_change BEFORE UPDATE OR DELETE ON admin_i3_requests
    FOR EACH ROW EXECUTE FUNCTION reject_allocation_delete();
