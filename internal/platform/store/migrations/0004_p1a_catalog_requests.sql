CREATE TABLE platform_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    accepting_new_requests boolean NOT NULL DEFAULT true,
    maintenance_message text NOT NULL DEFAULT ''
);
INSERT INTO platform_settings(singleton) VALUES(true);

CREATE TABLE arcade_games (
    id uuid PRIMARY KEY,
    workshop_id text NOT NULL UNIQUE CHECK (workshop_id ~ '^[0-9]+$'),
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
    enabled boolean NOT NULL DEFAULT true,
    accepting_new_requests boolean NOT NULL DEFAULT true,
    maintenance_message text NOT NULL DEFAULT '',
    current_content_version_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE content_versions (
    id text PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 128),
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (arcade_game_id,id)
);
ALTER TABLE arcade_games ADD CONSTRAINT arcade_current_content_version
    FOREIGN KEY (id,current_content_version_id)
    REFERENCES content_versions(arcade_game_id,id) DEFERRABLE INITIALLY IMMEDIATE;

CREATE FUNCTION reject_content_version_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'ContentVersion is immutable';
END;
$$;
CREATE TRIGGER content_version_immutable BEFORE UPDATE OR DELETE ON content_versions
    FOR EACH ROW EXECUTE FUNCTION reject_content_version_change();

CREATE TABLE template_revisions (
    id text PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 128),
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (arcade_game_id,id)
);

CREATE TABLE game_presets (
    id uuid PRIMARY KEY,
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
    enabled boolean NOT NULL DEFAULT true,
    accepting_new_requests boolean NOT NULL DEFAULT true,
    maintenance_message text NOT NULL DEFAULT '',
    max_players integer NOT NULL CHECK (max_players > 0),
    template_revision_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id,arcade_game_id),
    FOREIGN KEY (arcade_game_id,template_revision_id)
        REFERENCES template_revisions(arcade_game_id,id)
);

CREATE TABLE node_template_bindings (
    node_id uuid NOT NULL REFERENCES nodes(id),
    template_revision_id text NOT NULL REFERENCES template_revisions(id),
    binding_key text NOT NULL CHECK (length(binding_key) BETWEEN 1 AND 128),
    PRIMARY KEY (node_id,template_revision_id)
);

CREATE TABLE server_requests (
    id uuid PRIMARY KEY,
    owner_user_id uuid REFERENCES users(id),
    owner_party_id uuid,
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    game_preset_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'waiting' CHECK (state IN
        ('waiting','allocating','creating','running','stopping','ended','cancelled','unavailable','quarantined')),
    requested_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(owner_user_id,owner_party_id)=1),
    FOREIGN KEY (game_preset_id,arcade_game_id) REFERENCES game_presets(id,arcade_game_id)
);
CREATE UNIQUE INDEX server_requests_one_blocking_user ON server_requests(owner_user_id)
    WHERE owner_user_id IS NOT NULL AND state IN ('waiting','allocating','creating','running','stopping');
CREATE UNIQUE INDEX server_requests_one_blocking_party ON server_requests(owner_party_id)
    WHERE owner_party_id IS NOT NULL AND state IN ('waiting','allocating','creating','running','stopping');
CREATE INDEX server_requests_owner_history ON server_requests(owner_user_id,requested_at DESC);
