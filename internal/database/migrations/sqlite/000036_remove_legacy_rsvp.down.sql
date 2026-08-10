-- Migration 36 deletes legacy data that cannot be reconstructed from the Gate
-- 2 schema. This deliberately fails before any schema mutation. Restore the
-- verified pre-upgrade database backup instead.
SELECT owl_invites_migration_36_is_irreversible_restore_preupgrade_backup();
