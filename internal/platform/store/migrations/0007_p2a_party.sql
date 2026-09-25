-- Deployment must explicitly set this positive value before Party creation.
-- Migration deliberately does not infer it from any GamePreset.
ALTER TABLE platform_settings ADD COLUMN max_party_size integer CHECK (max_party_size > 0);

CREATE TABLE parties (
    id uuid PRIMARY KEY,
    leader_user_id uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    dissolved_at timestamptz,
    CHECK ((dissolved_at IS NULL) = (leader_user_id IS NOT NULL))
);

CREATE TABLE party_members (
    party_id uuid NOT NULL REFERENCES parties(id),
    user_id uuid NOT NULL REFERENCES users(id),
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (party_id,user_id),
    UNIQUE (user_id)
);

-- A live Party cannot exist without its leader's membership. Deferred for
-- the initial Party + leader member insertion in one transaction.
ALTER TABLE parties ADD CONSTRAINT party_leader_is_member
    FOREIGN KEY (id,leader_user_id) REFERENCES party_members(party_id,user_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE party_invites (
    id uuid PRIMARY KEY,
    party_id uuid NOT NULL REFERENCES parties(id),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
    -- The current link is retrievable by the leader. The token is only returned
    -- by a leader-authorized endpoint and must never appear in logs.
    token_value text NOT NULL CHECK (length(token_value)=43),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE UNIQUE INDEX party_invites_one_current ON party_invites(party_id)
    WHERE revoked_at IS NULL;

ALTER TABLE server_requests ADD CONSTRAINT server_request_party_owner
    FOREIGN KEY (owner_party_id) REFERENCES parties(id);
CREATE INDEX server_requests_party_history ON server_requests(owner_party_id,requested_at DESC);
