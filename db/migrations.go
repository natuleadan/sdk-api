package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Migration is one versioned SQL file. The version is the leading integer of the
// file name (e.g. 0002_add_users_email.sql -> 2), so order is explicit and never
// depends on directory listing.
type Migration struct {
	Version  int
	Name     string
	Path     string
	SQL      string
	Checksum string
}

// MigrationStatus describes one migration against the applied set.
type MigrationStatus struct {
	Migration
	Applied  bool
	Modified bool // applied before, but the file checksum changed
}

// Migrator applies versioned SQL migrations to a driver-agnostic *sql.DB. It
// keeps a schema_migrations control table so every environment (dev, staging,
// prod) converges to the same schema by running only the pending files, in
// order. This is the ALTER-table companion to AutoInit: AutoInit creates a table
// that does not exist, the migrator evolves one that does.
type Migrator struct {
	db         *sql.DB
	root       *os.Root
	migrations []Migration
}

var migrationFileRe = regexp.MustCompile(`^(\d+)[_-](.+)\.sql$`)

// NewMigrator loads every migration file from dir. Files must be named
// <version>_<name>.sql (e.g. 0001_init.sql); the version is the leading number.
// File access is confined to dir with os.Root, so a name can never escape the
// migrations directory.
func NewMigrator(database *sql.DB, dir string) (*Migrator, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("db: open migrations dir %q: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("db: read migrations dir %q: %w", dir, err)
	}
	var migrations []Migration
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		// Down files are companions of an up migration, not versions of their
		// own (0001_x.down.sql shares the version of 0001_x.sql).
		if strings.HasSuffix(m[2], ".down") {
			continue
		}
		version := atoi(m[1])
		if prev, ok := seen[version]; ok {
			_ = root.Close()
			return nil, fmt.Errorf("db: duplicate migration version %d (%q and %q)", version, prev, e.Name())
		}
		seen[version] = e.Name()
		raw, err := root.ReadFile(e.Name())
		if err != nil {
			_ = root.Close()
			return nil, fmt.Errorf("db: read %q: %w", e.Name(), err)
		}
		sum := sha256.Sum256(raw)
		migrations = append(migrations, Migration{
			Version:  version,
			Name:     m[2],
			Path:     filepath.Join(dir, e.Name()),
			SQL:      string(raw),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return &Migrator{db: database, root: root, migrations: migrations}, nil
}

// Close releases the migrations directory handle.
func (m *Migrator) Close() error {
	if m.root == nil {
		return nil
	}
	return m.root.Close()
}

// Migrations returns the loaded migrations (ordered).
func (m *Migrator) Migrations() []Migration { return m.migrations }

// ensureTable creates the control table if missing. It is dialect-neutral DDL
// that all three families (postgres, mysql, sqlite/turso) accept.
func (m *Migrator) ensureTable(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("db: ensure schema_migrations: %w", err)
	}
	return nil
}

// applied returns the versions already recorded with their checksums.
func (m *Migrator) applied(ctx context.Context) (map[int]string, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("db: read schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int]string{}
	for rows.Next() {
		var v int
		var sum string
		if err := rows.Scan(&v, &sum); err != nil {
			return nil, fmt.Errorf("db: scan schema_migrations: %w", err)
		}
		out[v] = sum
	}
	return out, rows.Err()
}

// Status reports every loaded migration against what the database has applied,
// flagging applied files whose checksum changed (a schema drift warning).
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	applied, err := m.applied(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MigrationStatus, 0, len(m.migrations))
	for _, mig := range m.migrations {
		st := MigrationStatus{Migration: mig}
		if sum, ok := applied[mig.Version]; ok {
			st.Applied = true
			st.Modified = sum != mig.Checksum
		}
		out = append(out, st)
	}
	return out, nil
}

// Up applies every pending migration in order. It returns the versions applied
// in this run. A migration whose file changed after being applied is a hard
// error: the schema and the file would diverge, so the run stops instead of
// guessing.
func (m *Migrator) Up(ctx context.Context) ([]int, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	applied, err := m.applied(ctx)
	if err != nil {
		return nil, err
	}
	var done []int
	for _, mig := range m.migrations {
		if sum, ok := applied[mig.Version]; ok {
			if sum != mig.Checksum {
				return done, fmt.Errorf("db: migration %d (%s) changed after being applied (checksum mismatch)", mig.Version, mig.Name)
			}
			continue
		}
		if err := m.applyOne(ctx, mig); err != nil {
			return done, err
		}
		done = append(done, mig.Version)
	}
	return done, nil
}

// applyOne runs a single migration and records it, inside one transaction so a
// failure leaves neither the schema nor the control table half-updated.
func (m *Migrator) applyOne(ctx context.Context, mig Migration) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin migration %d: %w", mig.Version, err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range splitStatements(mig.SQL) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("db: migration %d (%s): %w", mig.Version, mig.Name, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES ($1,$2,$3,$4)`,
		mig.Version, mig.Name, mig.Checksum, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("db: record migration %d: %w", mig.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit migration %d: %w", mig.Version, err)
	}
	return nil
}

// Down rolls back the highest applied migration using its .down.sql companion
// (e.g. 0002_add_users_email.down.sql). Missing companion is an error: an
// irreversible migration must be explicit, never silently skipped.
func (m *Migrator) Down(ctx context.Context) (int, error) {
	if err := m.ensureTable(ctx); err != nil {
		return 0, err
	}
	applied, err := m.applied(ctx)
	if err != nil {
		return 0, err
	}
	// Highest applied migration (not simply the last file: there may be pending
	// versions above it).
	var last *Migration
	for i := range m.migrations {
		if _, ok := applied[m.migrations[i].Version]; ok {
			last = &m.migrations[i]
		}
	}
	if last == nil {
		return 0, nil
	}
	downName := fmt.Sprintf("%0*d_%s.down.sql", 4, last.Version, last.Name)
	raw, err := m.root.ReadFile(downName)
	if err != nil {
		return 0, fmt.Errorf("db: migration %d has no down file (%s)", last.Version, downName)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("db: begin rollback %d: %w", last.Version, err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range splitStatements(string(raw)) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return 0, fmt.Errorf("db: rollback %d: %w", last.Version, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, last.Version); err != nil {
		return 0, fmt.Errorf("db: unrecord %d: %w", last.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("db: commit rollback %d: %w", last.Version, err)
	}
	return last.Version, nil
}

// splitStatements splits a migration file on semicolons at line ends, dropping
// comments and blank fragments. It keeps the runner dependency-free: migrations
// are plain SQL, one statement per block, never stored procedures with embedded
// semicolons.
func splitStatements(sqlText string) []string {
	var out []string
	var b strings.Builder
	for line := range strings.SplitSeq(sqlText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			stmt := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(b.String()), ";"))
			if stmt != "" {
				out = append(out, stmt)
			}
			b.Reset()
		}
	}
	if stmt := strings.TrimSpace(b.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}

// atoi parses a decimal version prefix; an unparsable prefix yields 0, which the
// duplicate check treats consistently.
func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
