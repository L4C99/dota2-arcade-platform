-- P2A persisted leader identity on Party. Materialize the same immutable V1
-- role on PartyMember and require the Party leader to reference that row.
ALTER TABLE party_members ADD COLUMN role text NOT NULL DEFAULT 'member'
    CHECK (role IN ('leader','member'));

UPDATE party_members m SET role='leader'
FROM parties p WHERE m.party_id=p.id AND m.user_id=p.leader_user_id
    AND p.dissolved_at IS NULL;

CREATE UNIQUE INDEX party_members_one_leader ON party_members(party_id)
    WHERE role='leader';
ALTER TABLE party_members ADD CONSTRAINT party_members_role_reference UNIQUE (party_id,user_id,role);
ALTER TABLE parties ADD COLUMN leader_role text NOT NULL DEFAULT 'leader' CHECK (leader_role='leader');
ALTER TABLE parties ADD CONSTRAINT party_leader_has_leader_role
    FOREIGN KEY (id,leader_user_id,leader_role)
    REFERENCES party_members(party_id,user_id,role)
    DEFERRABLE INITIALLY DEFERRED;
