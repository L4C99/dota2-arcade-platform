ALTER TABLE node_entry_capabilities
    ADD COLUMN steam_verified_ports integer[] NOT NULL DEFAULT '{}',
    ADD COLUMN steamchina_verified_ports integer[] NOT NULL DEFAULT '{}',
    ADD COLUMN steam_verification_note text NOT NULL DEFAULT '',
    ADD COLUMN steamchina_verification_note text NOT NULL DEFAULT '';
-- Older verification records did not prove complete port coverage. Require fresh
-- human verification before either scheme can be shown to players again.
UPDATE node_entry_capabilities SET
    steam_entry_verified=false, steam_entry_enabled=false,
    steam_verified_at=NULL, steam_verified_by=NULL,
    steamchina_entry_verified=false, steamchina_entry_enabled=false,
    steamchina_verified_at=NULL, steamchina_verified_by=NULL;
ALTER TABLE node_entry_capabilities
    ADD CONSTRAINT steam_coverage_requires_verification CHECK (steam_entry_verified = (cardinality(steam_verified_ports)>0)),
    ADD CONSTRAINT steamchina_coverage_requires_verification CHECK (steamchina_entry_verified = (cardinality(steamchina_verified_ports)>0));
