-- Migration 15 remains immutable history. Amendment 001 removes its obsolete
-- port-coverage requirement without changing existing verified/enabled facts.
ALTER TABLE node_entry_capabilities
    DROP CONSTRAINT steam_coverage_requires_verification,
    DROP CONSTRAINT steamchina_coverage_requires_verification,
    DROP COLUMN steam_verified_ports,
    DROP COLUMN steamchina_verified_ports,
    DROP COLUMN steam_verification_note,
    DROP COLUMN steamchina_verification_note;
