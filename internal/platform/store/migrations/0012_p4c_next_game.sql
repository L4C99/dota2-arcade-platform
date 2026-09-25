CREATE TABLE next_game_intents (
    source_request_id uuid PRIMARY KEY REFERENCES server_requests(id),
    source_allocation_id uuid NOT NULL UNIQUE REFERENCES allocations(id),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','paused','consumed')),
    new_request_id uuid UNIQUE REFERENCES server_requests(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    consumed_at timestamptz,
    CHECK ((state = 'consumed') = (new_request_id IS NOT NULL)),
    CHECK ((state = 'consumed') = (consumed_at IS NOT NULL))
);
