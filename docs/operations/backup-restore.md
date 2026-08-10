# Backup, restore, and disaster recovery

A complete Owl Invites recovery point has three separately protected parts:

```text
database + uploads + OWL_INVITES_SECRET_KEY
```

The key must not be stored in the ordinary database/upload bundle, manifest,
logs, SBOM, provenance, or build artifact. Store it in an independent secret
manager/offline recovery system. Backup metadata records only:

```text
HMAC-SHA256(secret, "owl-invites-secret-fingerprint:v1")[:16]
```

encoded as `oi-secret-v1:<32 lowercase hex characters>`. It detects a mismatch
and is never accepted for authentication.

```bash
owl-invites secret fingerprint
```

## SQLite

### Create

Run the CLI with the live SQLite database, uploads directory, and secret
configuration available:

```bash
owl-invites backup sqlite --output /backups/owl-invites-2026-08-10T120000Z
```

The output is a new mode-0700 directory. The command refuses to overwrite any
path, refuses dirty migration state, refuses an output inside `UPLOADS_DIR`,
and rejects upload symlinks/non-regular files. It contains:

```text
manifest.json       mode 0600
database.sqlite     mode 0600
uploads/            mode 0700; copied files mode 0600
```

`database.sqlite` is created with SQLite `VACUUM INTO`, which SQLite documents
as a transactional consistent snapshot of a live database, including WAL-mode
sources. It is not a byte-copy of the live main file.

The version-1 manifest records UTC creation time, SQLite/schema state,
application version/commit/build state, database size/SHA-256, secret
fingerprint, and a path/size/SHA-256 inventory of every upload. It explicitly
marks the upload copy as a filesystem snapshot taken after the database and
not transactionally atomic. Quiesce upload mutations while backing up when an
exact database/filesystem cut is required.

### Verify

Copy the bundle away from the live volume and verify the copy:

```bash
owl-invites backup verify /backups/owl-invites-2026-08-10T120000Z
```

Verification rejects unexpected top-level files, symlinks, malformed or
unknown manifest fields, checksum/size/inventory changes, dirty/schema
mismatch, failed SQLite `quick_check`, and foreign-key violations. It does not
need the secret and therefore can run in isolated backup infrastructure.

### Restore acceptance and procedure

Stop the application. Restore only into new paths; the CLI intentionally will
not overwrite an existing database or uploads directory:

```bash
export DB_DRIVER=sqlite
export OWL_INVITES_SECRET_KEY_FILE=/secure/owl-invites-capability-key
owl-invites backup restore /backups/owl-invites-2026-08-10T120000Z \
  --database /restore/openrsvp.db \
  --uploads /restore/uploads
```

Restore first performs full verification, compares the active key fingerprint,
stages and rechecks the database, stages uploads, and commits both new targets.
A wrong key fails before either target is created. After restore:

1. point a stopped/candidate instance at the new database and uploads;
2. run `owl-invites migrate status` (do not migrate unexpectedly during a
   forensic restore);
3. start the intended application version;
4. require `/health/ready` to return 200;
5. exchange a known pre-backup household capability; and
6. confirm its guests, response, question answers, and representative uploads.

CI performs this exact drill, including capability failure under a different
secret and corruption detection after an upload is modified.

## PostgreSQL

Use PostgreSQL's custom archive format. Use a `pg_dump` client of the same major
version as the server (or a supported newer client); do not use an older client
against a newer server. Restore with the matching `pg_restore` toolchain.

Coordinate a recovery point, quiescing application/upload mutations for the
strongest cross-store consistency, then run:

```bash
pg_dump --format=custom --no-owner --no-privileges \
  --file database.dump "$DB_DSN"
pg_restore --list database.dump > database.list
test -s database.list
tar -C "$UPLOADS_DIR" -cf uploads.tar .
sha256sum database.dump uploads.tar
owl-invites migrate version
owl-invites secret fingerprint
owl-invites version --json
```

Record the checksums, schema version, key fingerprint, build identity,
`pg_dump --version`, and the same non-atomic upload timing statement beside the
artifacts. Do not record the raw DSN if it contains credentials and never place
the capability key in that metadata/archive.

Restore into a clean database and new upload directory while the application
is stopped:

```bash
createdb owl_invites_restored
pg_restore --exit-on-error --no-owner --no-privileges \
  --dbname "$RESTORE_DSN" database.dump
mkdir /restore/uploads
tar --extract --no-same-owner -f uploads.tar -C /restore/uploads
```

Recompute checksums and the active key fingerprint before start. Then use the
same readiness/capability/state validation as SQLite. The automated PostgreSQL
16 drill seeds real Gate 2 state, drops the source database, restores a clean
database, restores uploads separately, validates the old capability through
the HTTP API, and proves a different key yields the same generic 401 as a
missing invitation while readiness remains healthy.

## Disaster decisions

| Failure | Required response |
| --- | --- |
| Database lost | Restore a verified database plus matching uploads/key; never initialize over the only copy. |
| Uploads lost | Restore the separately checksummed upload snapshot; database rows may otherwise reference missing assets. |
| Capability key lost | Old private/open/recovery capabilities cannot be reconstructed. Restore the separately protected original key or accept global capability invalidation and perform an explicit recovery campaign. |
| Wrong key paired with database | Stop/correct the deployment. Compare `owl-invites secret fingerprint` with backup metadata. The database can be healthy while every old capability correctly fails. |
| SQLite verification/corruption failure | Do not restore or repair in place. Preserve the failed artifact and select an earlier verified recovery point. |
| `pg_restore` failure | Discard the incomplete target database, correct version/ownership/extension issues, create another clean target, and rerun with `--exit-on-error`. |
| Migration failure before 36 | Preserve logs/state and restore the verified pre-upgrade recovery point before retrying. |
| Rollback across migration 36 | Restore the full verified pre-Gate-2 database/uploads/key and run the older application. Migration-down is deliberately unsupported. |

Backups are not valid merely because a command exited zero. Retain periodic
automated restore drills and record the exact application digest used to prove
readiness and capability continuity.
