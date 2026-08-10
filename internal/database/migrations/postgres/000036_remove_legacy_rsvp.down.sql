-- Migration 36 deletes legacy data that cannot be reconstructed from the Gate
-- 2 schema. This deliberately fails before any schema mutation. Restore the
-- verified pre-upgrade database backup instead.
DO $$
BEGIN
    RAISE EXCEPTION 'migration 36 is irreversible: restore a verified pre-upgrade backup to roll back Gate 2';
END
$$;
