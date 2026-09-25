ALTER TABLE nodes ADD COLUMN priority integer NOT NULL DEFAULT 0;

ALTER TABLE server_requests
    ADD COLUMN node_selection_mode text NOT NULL DEFAULT 'auto'
        CHECK (node_selection_mode IN ('auto','manual')),
    ADD COLUMN manual_node_id uuid REFERENCES nodes(id),
    ADD CONSTRAINT server_requests_manual_node_required CHECK
        ((node_selection_mode='auto' AND manual_node_id IS NULL)
         OR (node_selection_mode='manual' AND manual_node_id IS NOT NULL));

CREATE INDEX server_requests_waiting_fifo ON server_requests(requested_at,id)
    WHERE state='waiting';
