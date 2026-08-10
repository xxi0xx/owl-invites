package backup

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/yannkr/openrsvp/internal/buildinfo"
	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/secretkey"
)

// CreateSQLite creates a restricted directory bundle containing a SQLite
// native snapshot, upload snapshot, and non-secret verification metadata.
func CreateSQLite(ctx context.Context, db database.DB, uploadsDir, destination, secret string) (*Manifest, error) {
	if db.Dialect() != "sqlite" {
		return nil, fmt.Errorf("sqlite backup requires DB_DRIVER=sqlite")
	}
	status, err := database.ReadMigrationStatus(db)
	if err != nil {
		return nil, fmt.Errorf("read source schema: %w", err)
	}
	if status.Dirty {
		return nil, fmt.Errorf("refusing to back up a dirty migration state at version %d", status.Current)
	}
	if _, err := os.Stat(uploadsDir); err != nil {
		return nil, fmt.Errorf("read uploads directory: %w", err)
	}

	destination, err = filepath.Abs(destination)
	if err != nil {
		return nil, err
	}
	if err := requireAbsent(destination); err != nil {
		return nil, err
	}
	uploadsAbsolute, err := filepath.Abs(uploadsDir)
	if err != nil {
		return nil, err
	}
	if pathWithin(destination, uploadsAbsolute) {
		return nil, fmt.Errorf("backup destination must not be inside the uploads directory")
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(parent, ".owl-invites-backup-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(stage, 0o700); err != nil {
		_ = os.RemoveAll(stage)
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()

	databasePath := filepath.Join(stage, databaseFilename)
	if _, err := db.Underlying().ExecContext(ctx, "VACUUM INTO ?", databasePath); err != nil {
		return nil, fmt.Errorf("create SQLite online snapshot: %w", err)
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		return nil, err
	}
	version, dirty, err := inspectSQLite(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("verify SQLite snapshot: %w", err)
	}
	if version != status.Current || dirty != status.Dirty {
		return nil, fmt.Errorf("snapshot schema state changed: got version %d dirty=%t", version, dirty)
	}
	databaseArtifact, err := hashFile(databasePath, databaseFilename)
	if err != nil {
		return nil, err
	}

	uploadsPath := filepath.Join(stage, uploadsDirectory)
	if err := copyTree(uploadsDir, uploadsPath); err != nil {
		return nil, fmt.Errorf("snapshot uploads: %w", err)
	}
	uploads, err := inventoryUploads(uploadsPath)
	if err != nil {
		return nil, fmt.Errorf("inventory uploads: %w", err)
	}
	build := buildinfo.Current()
	manifest := &Manifest{
		FormatVersion: FormatVersion,
		CreatedAt:     time.Now().UTC(),
		Source: SourceMetadata{
			DatabaseType: "sqlite", SchemaVersion: version, SchemaDirty: dirty,
			ApplicationVersion: build.Version, Commit: build.Commit, BuildState: build.BuildState,
		},
		Database:          databaseArtifact,
		SecretFingerprint: secretkey.Fingerprint(secret),
		Uploads:           uploads,
	}
	if err := writeManifest(filepath.Join(stage, manifestFilename), manifest); err != nil {
		return nil, err
	}
	if err := requireAbsent(destination); err != nil {
		return nil, err
	}
	if err := os.Rename(stage, destination); err != nil {
		return nil, fmt.Errorf("commit backup bundle: %w", err)
	}
	committed = true
	return manifest, nil
}

// VerifySQLite validates bundle structure, every checksum, and SQLite
// quick-check, foreign keys, and schema metadata without requiring the secret.
func VerifySQLite(ctx context.Context, bundle string) (*Manifest, error) {
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return nil, err
	}
	if err := validateBundleLayout(bundle); err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(bundle, manifestFilename)
	if err := requireRegularFile(manifestPath); err != nil {
		return nil, fmt.Errorf("read backup manifest: %w", err)
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read backup manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	databasePath := filepath.Join(bundle, databaseFilename)
	if err := requireRegularFile(databasePath); err != nil {
		return nil, fmt.Errorf("read backup database: %w", err)
	}
	actualDatabase, err := hashFile(databasePath, databaseFilename)
	if err != nil {
		return nil, fmt.Errorf("read backup database: %w", err)
	}
	if actualDatabase != manifest.Database {
		return nil, fmt.Errorf("database checksum or size does not match manifest")
	}
	version, dirty, err := inspectSQLite(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("verify SQLite database: %w", err)
	}
	if version != manifest.Source.SchemaVersion || dirty != manifest.Source.SchemaDirty {
		return nil, fmt.Errorf("database schema state does not match manifest")
	}
	actualUploads, err := inventoryUploads(filepath.Join(bundle, uploadsDirectory))
	if err != nil {
		return nil, fmt.Errorf("verify uploads: %w", err)
	}
	if !uploadsEqual(manifest.Uploads, actualUploads) {
		return nil, fmt.Errorf("uploads checksum, size, or file inventory does not match manifest")
	}
	return manifest, nil
}

// RestoreSQLite verifies and restores a bundle to new database/upload paths.
// It refuses to overwrite either destination and requires the matching secret.
func RestoreSQLite(ctx context.Context, bundle, databaseDestination, uploadsDestination, secret string) (*Manifest, error) {
	manifest, err := VerifySQLite(ctx, bundle)
	if err != nil {
		return nil, err
	}
	activeFingerprint := secretkey.Fingerprint(secret)
	if activeFingerprint != manifest.SecretFingerprint {
		return nil, fmt.Errorf("secret fingerprint mismatch: backup=%s active=%s", manifest.SecretFingerprint, activeFingerprint)
	}
	databaseDestination, err = filepath.Abs(databaseDestination)
	if err != nil {
		return nil, err
	}
	uploadsDestination, err = filepath.Abs(uploadsDestination)
	if err != nil {
		return nil, err
	}
	if pathsOverlap(databaseDestination, uploadsDestination) {
		return nil, fmt.Errorf("database destination must not be inside uploads destination")
	}
	bundleAbsolute, err := filepath.Abs(bundle)
	if err != nil {
		return nil, err
	}
	if pathWithin(databaseDestination, bundleAbsolute) || pathWithin(uploadsDestination, bundleAbsolute) {
		return nil, fmt.Errorf("restore destinations must not be inside the backup bundle")
	}
	if err := requireAbsent(databaseDestination); err != nil {
		return nil, err
	}
	if err := requireAbsent(uploadsDestination); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(databaseDestination), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(uploadsDestination), 0o700); err != nil {
		return nil, err
	}

	databaseStage, err := os.MkdirTemp(filepath.Dir(databaseDestination), ".owl-invites-db-restore-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(databaseStage) }()
	databaseTemp := filepath.Join(databaseStage, databaseFilename)
	if err := copyRegularFile(filepath.Join(bundle, databaseFilename), databaseTemp); err != nil {
		return nil, err
	}
	if _, _, err := inspectSQLite(ctx, databaseTemp); err != nil {
		return nil, fmt.Errorf("verify restored database staging file: %w", err)
	}

	uploadsStage, err := os.MkdirTemp(filepath.Dir(uploadsDestination), ".owl-invites-uploads-restore-")
	if err != nil {
		return nil, err
	}
	uploadsCommitted := false
	defer func() {
		if !uploadsCommitted {
			_ = os.RemoveAll(uploadsStage)
		}
	}()
	if err := copyTree(filepath.Join(bundle, uploadsDirectory), uploadsStage); err != nil {
		return nil, err
	}
	if err := requireAbsent(databaseDestination); err != nil {
		return nil, err
	}
	if err := requireAbsent(uploadsDestination); err != nil {
		return nil, err
	}
	if err := os.Rename(databaseTemp, databaseDestination); err != nil {
		return nil, fmt.Errorf("commit restored database: %w", err)
	}
	if err := os.Rename(uploadsStage, uploadsDestination); err != nil {
		_ = os.Remove(databaseDestination)
		return nil, fmt.Errorf("commit restored uploads: %w", err)
	}
	uploadsCommitted = true
	return manifest, nil
}

func inspectSQLite(ctx context.Context, path string) (uint, bool, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return 0, false, err
	}
	uri := &url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := url.Values{}
	query.Set("mode", "ro")
	query.Set("_foreign_keys", "on")
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(ctx, "PRAGMA quick_check")
	if err != nil {
		return 0, false, err
	}
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			_ = rows.Close()
			return 0, false, err
		}
		if result != "ok" {
			_ = rows.Close()
			return 0, false, fmt.Errorf("quick_check reported corruption")
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, false, err
	}
	if err := rows.Close(); err != nil {
		return 0, false, err
	}

	foreignRows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return 0, false, err
	}
	if foreignRows.Next() {
		_ = foreignRows.Close()
		return 0, false, fmt.Errorf("foreign_key_check reported a violation")
	}
	if err := foreignRows.Err(); err != nil {
		_ = foreignRows.Close()
		return 0, false, err
	}
	if err := foreignRows.Close(); err != nil {
		return 0, false, err
	}

	var version uint
	var dirty bool
	err = db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return version, dirty, nil
}

func writeManifest(path string, manifest *Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write backup manifest: %w", err)
	}
	return nil
}

func readManifest(path string) (*Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > 1<<20 {
		return nil, fmt.Errorf("manifest exceeds 1 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("manifest contains trailing data")
	}
	return &manifest, nil
}

func validateManifest(manifest *Manifest) error {
	if manifest.FormatVersion != FormatVersion {
		return fmt.Errorf("unsupported backup format version %d", manifest.FormatVersion)
	}
	if manifest.CreatedAt.IsZero() || manifest.Source.DatabaseType != "sqlite" || manifest.Source.SchemaDirty {
		return fmt.Errorf("invalid or dirty SQLite backup manifest")
	}
	if manifest.Database.Path != databaseFilename || manifest.Database.Size < 0 || len(manifest.Database.SHA256) != 64 {
		return fmt.Errorf("invalid database artifact metadata")
	}
	if _, err := hex.DecodeString(manifest.Database.SHA256); err != nil {
		return fmt.Errorf("invalid database checksum")
	}
	if !strings.HasPrefix(manifest.SecretFingerprint, "oi-secret-v1:") || len(manifest.SecretFingerprint) != len("oi-secret-v1:")+32 {
		return fmt.Errorf("invalid secret fingerprint metadata")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(manifest.SecretFingerprint, "oi-secret-v1:")); err != nil {
		return fmt.Errorf("invalid secret fingerprint metadata")
	}
	if !manifest.Uploads.Included || manifest.Uploads.Consistency != uploadsConsistency || manifest.Uploads.FileCount != len(manifest.Uploads.Files) {
		return fmt.Errorf("invalid uploads metadata")
	}
	for _, file := range manifest.Uploads.Files {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path)))
		if file.Path == "" || clean != file.Path || strings.HasPrefix(clean, "../") || filepath.IsAbs(filepath.FromSlash(file.Path)) || len(file.SHA256) != 64 || file.Size < 0 {
			return fmt.Errorf("invalid upload artifact metadata")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return fmt.Errorf("invalid upload checksum")
		}
	}
	return nil
}

func requireAbsent(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return fmt.Errorf("refusing to overwrite existing path: %s", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func requireRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("expected a regular file: %s", path)
	}
	return nil
}

func validateBundleLayout(bundle string) error {
	info, err := os.Lstat(bundle)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("backup bundle must be a directory, not a symbolic link")
	}
	entries, err := os.ReadDir(bundle)
	if err != nil {
		return err
	}
	want := map[string]bool{manifestFilename: false, databaseFilename: false, uploadsDirectory: true}
	if len(entries) != len(want) {
		return fmt.Errorf("backup bundle contains unexpected top-level entries")
	}
	for _, entry := range entries {
		expectDirectory, ok := want[entry.Name()]
		if !ok || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() != expectDirectory {
			return fmt.Errorf("backup bundle has invalid top-level layout")
		}
	}
	return nil
}

func pathsOverlap(databasePath, uploadsPath string) bool {
	return pathWithin(databasePath, uploadsPath)
}

func pathWithin(path, parent string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
