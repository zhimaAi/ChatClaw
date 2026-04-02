package settings

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func TestParseVecDimension(t *testing.T) {
	t.Parallel()

	got := parseVecDimension(`CREATE VIRTUAL TABLE "doc_vec" USING vec0(id INTEGER PRIMARY KEY, content FLOAT[1024]);`)
	if got != 1024 {
		t.Fatalf("parseVecDimension() = %d, want 1024", got)
	}
}

func TestInspectDocVecStateClean(t *testing.T) {
	t.Parallel()

	db := newTestBunDB(t)
	ctx := context.Background()

	mustExec(t, db.DB, `CREATE TABLE doc_vec (id INTEGER PRIMARY KEY, content FLOAT[1024]);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_chunks (chunk_id INTEGER PRIMARY KEY, size INTEGER NOT NULL, validity BLOB NOT NULL, rowids BLOB NOT NULL);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_info (key TEXT PRIMARY KEY, value ANY);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_rowids (rowid INTEGER PRIMARY KEY, id INTEGER, chunk_id INTEGER, chunk_offset INTEGER);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_vector_chunks00 (rowid INTEGER PRIMARY KEY, vectors BLOB NOT NULL);`)
	mustExec(t, db.DB, `INSERT INTO doc_vec_vector_chunks00(rowid, vectors) VALUES (1, zeroblob(?));`, 1024*4*1024)

	needsRepair, reason, err := inspectDocVecState(ctx, db, 1024)
	if err != nil {
		t.Fatalf("inspectDocVecState() error = %v", err)
	}
	if needsRepair {
		t.Fatalf("inspectDocVecState() unexpectedly requested repair: %s", reason)
	}
}

func TestInspectDocVecStateDetectsStaleArtifactsAndBlobMismatch(t *testing.T) {
	t.Parallel()

	db := newTestBunDB(t)
	ctx := context.Background()

	mustExec(t, db.DB, `CREATE TABLE doc_vec (id INTEGER PRIMARY KEY, content FLOAT[1536]);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_chunks (chunk_id INTEGER PRIMARY KEY, size INTEGER NOT NULL, validity BLOB NOT NULL, rowids BLOB NOT NULL);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_info (key TEXT PRIMARY KEY, value ANY);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_rowids (rowid INTEGER PRIMARY KEY, id INTEGER, chunk_id INTEGER, chunk_offset INTEGER);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_vector_chunks00 (rowid INTEGER PRIMARY KEY, vectors BLOB NOT NULL);`)
	mustExec(t, db.DB, `CREATE TABLE doc_vec_old_123 (id INTEGER PRIMARY KEY);`)
	mustExec(t, db.DB, `INSERT INTO doc_vec_vector_chunks00(rowid, vectors) VALUES (1, zeroblob(?));`, 1024*4*1024)

	needsRepair, reason, err := inspectDocVecState(ctx, db, 1536)
	if err != nil {
		t.Fatalf("inspectDocVecState() error = %v", err)
	}
	if !needsRepair {
		t.Fatalf("inspectDocVecState() = false, want true")
	}
	if !strings.Contains(reason, "stale vec artifact") {
		t.Fatalf("reason %q does not mention stale artifact", reason)
	}
	if !strings.Contains(reason, "blob size") {
		t.Fatalf("reason %q does not mention blob mismatch", reason)
	}
}

func newTestBunDB(t *testing.T) *bun.DB {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open(): %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return bun.NewDB(sqlDB, sqlitedialect.New())
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q failed: %v", query, err)
	}
}
