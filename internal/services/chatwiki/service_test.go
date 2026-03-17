package chatwiki

import (
	"database/sql"
	"io"
	"log/slog"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/wailsapp/wails/v3/pkg/application"
	_ "unsafe"
)

//go:linkname testSQLiteDB chatclaw/internal/sqlite.db
var testSQLiteDB *bun.DB

func TestSaveBinding_PersistsChatWikiVersion(t *testing.T) {
	setChatWikiBindingTestDB(t)

	app := &application.App{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := SaveBinding(app, "https://example.com", "token-1", "7200", "4102444800", "user-1", "frog", "release"); err != nil {
		t.Fatalf("SaveBinding returned error: %v", err)
	}

	svc := &ChatWikiService{app: app}
	binding, err := svc.GetBinding()
	if err != nil {
		t.Fatalf("GetBinding returned error: %v", err)
	}
	if binding == nil {
		t.Fatal("expected binding, got nil")
	}
	if binding.ChatWikiVersion != "release" {
		t.Fatalf("expected chatwiki version release, got %#v", binding)
	}
}

func TestSaveBinding_DefaultsChatWikiVersionToDev(t *testing.T) {
	setChatWikiBindingTestDB(t)

	app := &application.App{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := SaveBinding(app, "https://example.com", "token-2", "7200", "4102444800", "user-2", "frog", ""); err != nil {
		t.Fatalf("SaveBinding returned error: %v", err)
	}

	svc := &ChatWikiService{app: app}
	binding, err := svc.GetBinding()
	if err != nil {
		t.Fatalf("GetBinding returned error: %v", err)
	}
	if binding == nil {
		t.Fatal("expected binding, got nil")
	}
	if binding.ChatWikiVersion != "dev" {
		t.Fatalf("expected default chatwiki version dev, got %#v", binding)
	}
}

func TestSaveBinding_AddsMissingChatWikiVersionColumnForLegacyDB(t *testing.T) {
	setLegacyChatWikiBindingTestDB(t)

	app := &application.App{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := SaveBinding(app, "https://example.com", "token-3", "7200", "4102444800", "user-3", "frog", "release"); err != nil {
		t.Fatalf("SaveBinding returned error: %v", err)
	}

	svc := &ChatWikiService{app: app}
	binding, err := svc.GetBinding()
	if err != nil {
		t.Fatalf("GetBinding returned error: %v", err)
	}
	if binding == nil {
		t.Fatal("expected binding, got nil")
	}
	if binding.ChatWikiVersion != "release" {
		t.Fatalf("expected chatwiki version release after legacy upgrade, got %#v", binding)
	}
}

func TestSaveBindingFromCallback_MapsCloudLoginSourceToYun(t *testing.T) {
	setChatWikiBindingTestDB(t)

	app := &application.App{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	svc := &ChatWikiService{app: app}
	if err := svc.SaveBindingFromCallback("https://example.com", "token-cloud", "7200", "4102444800", "user-cloud", "frog", "cloud"); err != nil {
		t.Fatalf("SaveBindingFromCallback returned error: %v", err)
	}

	binding, err := svc.GetBinding()
	if err != nil {
		t.Fatalf("GetBinding returned error: %v", err)
	}
	if binding == nil {
		t.Fatal("expected binding, got nil")
	}
	if binding.ChatWikiVersion != "yun" {
		t.Fatalf("expected chatwiki version yun, got %#v", binding)
	}
}

func TestSaveBindingFromCallback_DefaultsToDevWhenLoginSourceMissing(t *testing.T) {
	setChatWikiBindingTestDB(t)

	app := &application.App{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	svc := &ChatWikiService{app: app}
	if err := svc.SaveBindingFromCallback("https://example.com", "token-dev", "7200", "4102444800", "user-dev", "frog", ""); err != nil {
		t.Fatalf("SaveBindingFromCallback returned error: %v", err)
	}

	binding, err := svc.GetBinding()
	if err != nil {
		t.Fatalf("GetBinding returned error: %v", err)
	}
	if binding == nil {
		t.Fatal("expected binding, got nil")
	}
	if binding.ChatWikiVersion != "dev" {
		t.Fatalf("expected chatwiki version dev, got %#v", binding)
	}
}

func setChatWikiBindingTestDB(t *testing.T) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() {
		testSQLiteDB = nil
		_ = db.Close()
	})
	if _, err := db.Exec(`
create table chatwiki_bindings (
    id integer primary key autoincrement,
    server_url text not null default '',
    token text not null,
    ttl integer not null default 0,
    exp integer not null default 0,
    user_id text not null,
    user_name text not null default '',
    chatwiki_version text not null default 'dev',
    created_at datetime not null,
    updated_at datetime not null
)`); err != nil {
		t.Fatalf("create chatwiki_bindings table: %v", err)
	}
	testSQLiteDB = db
}

func setLegacyChatWikiBindingTestDB(t *testing.T) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() {
		testSQLiteDB = nil
		_ = db.Close()
	})
	if _, err := db.Exec(`
create table chatwiki_bindings (
    id integer primary key autoincrement,
    server_url text not null default '',
    token text not null,
    ttl integer not null default 0,
    exp integer not null default 0,
    user_id text not null,
    user_name text not null default '',
    created_at datetime not null,
    updated_at datetime not null
)`); err != nil {
		t.Fatalf("create legacy chatwiki_bindings table: %v", err)
	}
	testSQLiteDB = db
}
