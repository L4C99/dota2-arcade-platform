-- A human validation is recorded separately from Controller-reported disk facts.
-- It never changes a NodeContentBinding or the node's filesystem.
CREATE TABLE content_release_validations (
    node_id uuid NOT NULL REFERENCES nodes(id),
    arcade_game_id uuid NOT NULL REFERENCES arcade_games(id),
    content_version_id text NOT NULL,
    verified_by uuid NOT NULL REFERENCES admin_users(id),
    verified_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (node_id, arcade_game_id, content_version_id),
    FOREIGN KEY (arcade_game_id, content_version_id)
        REFERENCES content_versions(arcade_game_id, id)
);
