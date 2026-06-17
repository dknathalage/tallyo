-- +goose Up
-- The shifts lifecycle (00004) fully supersedes the per-participant notes
-- journal: shifts carry the same recorded supports plus a status lifecycle and
-- invoice linkage. No prod data to preserve (clean-break), so drop the table.
DROP TABLE notes;

-- +goose Down
-- Clean-break: notes is not restored on rollback. To recover the schema, see
-- 00003_notes.sql (the original CREATE TABLE notes + indexes).
