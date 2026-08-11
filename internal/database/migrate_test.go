package database

import (
	"path/filepath"
	"testing"

	"github.com/yannkr/openrsvp/internal/config"
)

func TestLatestMigrationVersion(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			got, err := LatestMigrationVersion(dialect)
			if err != nil {
				t.Fatal(err)
			}
			if got != 36 {
				t.Fatalf("latest version = %d, want 36", got)
			}
		})
	}
}

func TestMigrationStatusAndUpSQLite(t *testing.T) {
	db, err := New(&config.Config{DBDriver: "sqlite", DBDSN: filepath.Join(t.TempDir(), "migrate.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	before, err := ReadMigrationStatus(db)
	if err != nil {
		t.Fatal(err)
	}
	if before.Current != 0 || before.Latest != 36 || before.Dirty || !before.Pending {
		t.Fatalf("unexpected initial status: %+v", before)
	}

	result, err := MigrateUp(db)
	if err != nil {
		t.Fatal(err)
	}
	if result.Before != before {
		t.Fatalf("result before = %+v, want %+v", result.Before, before)
	}
	if result.After.Current != 36 || result.After.Latest != 36 || result.After.Dirty || result.After.Pending {
		t.Fatalf("unexpected migrated status: %+v", result.After)
	}

	second, err := MigrateUp(db)
	if err != nil {
		t.Fatal(err)
	}
	if second.Before != second.After {
		t.Fatalf("no-op migration changed status: %+v", second)
	}
}
