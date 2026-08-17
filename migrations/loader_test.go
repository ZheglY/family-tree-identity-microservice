package migrations

import "testing"

func TestLoadEmbeddedMigrations(t *testing.T) {
	migrations, err := Load(Files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := len(migrations), 1; got != want {
		t.Fatalf("migration count = %d, want %d", got, want)
	}

	migration := migrations[0]
	if got, want := migration.Version, int64(1); got != want {
		t.Fatalf("migration version = %d, want %d", got, want)
	}
	if migration.Name != "create_identity_schema" {
		t.Fatalf("migration name = %q", migration.Name)
	}
	if migration.UpSQL == "" || migration.DownSQL == "" {
		t.Fatal("migration SQL must not be empty")
	}
	if len(migration.Checksum) != 64 {
		t.Fatalf("checksum length = %d, want 64", len(migration.Checksum))
	}
}

func TestValidateAppliedRejectsChecksumDrift(t *testing.T) {
	migrations, err := Load(Files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	runner := &Runner{migrations: migrations}
	err = runner.validateApplied([]appliedMigration{
		{
			Version:  migrations[0].Version,
			Name:     migrations[0].Name,
			Checksum: "changed",
		},
	})
	if err == nil {
		t.Fatal("validateApplied() error = nil, want checksum error")
	}
}
