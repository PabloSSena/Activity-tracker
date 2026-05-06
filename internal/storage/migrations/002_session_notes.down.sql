-- SQLite < 3.35 cannot drop columns; recreate the table to revert.
-- This file is informational; the embed directive only includes *.up.sql.
ALTER TABLE sessions DROP COLUMN note;
