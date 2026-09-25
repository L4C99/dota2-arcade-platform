-- Presentation-only anonymous name. ApplyMigrations backfills existing Users
-- in this same transaction before setting NOT NULL and recording version 9.
ALTER TABLE users ADD COLUMN display_name text;
