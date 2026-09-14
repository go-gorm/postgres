package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// recordingPool captures the SQL issued through a gorm.ConnPool so migrator
// statements can be asserted without a live PostgreSQL.
type recordingPool struct {
	mu      sync.Mutex
	queries []string
}

func (p *recordingPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queries = append(p.queries, query)
	return nil, nil
}

func (p *recordingPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queries = append(p.queries, query)
	return driver.RowsAffected(0), nil
}

func (p *recordingPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queries = append(p.queries, query)
	return nil, nil
}

func (p *recordingPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return nil
}

func (p *recordingPool) recorded() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.queries...)
}

func openWithPool(t *testing.T, pool *recordingPool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(New(Config{Conn: pool}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open gorm with recording pool: %v", err)
	}
	return db
}

type dropIndexUser struct {
	ID uint
}

func (dropIndexUser) TableName() string { return "drop_index_users" }

type schemaQualifiedUser struct {
	ID uint
}

func (schemaQualifiedUser) TableName() string { return "public.drop_index_users" }

// Regression test for https://github.com/go-gorm/postgres/issues/350:
// without an explicit schema the migrator must not qualify the index with
// CURRENT_SCHEMA(), which is a function call and invalid as an identifier.
func TestDropAndRenameIndexWithoutExplicitSchema(t *testing.T) {
	pool := &recordingPool{}
	db := openWithPool(t, pool)

	if err := db.Migrator().DropIndex(&dropIndexUser{}, "idx_drop_index_users_name"); err != nil {
		t.Fatalf("DropIndex: %v", err)
	}
	if err := db.Migrator().RenameIndex(&dropIndexUser{}, "idx_old", "idx_new"); err != nil {
		t.Fatalf("RenameIndex: %v", err)
	}

	queries := pool.recorded()
	if len(queries) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(queries), queries)
	}
	if want := `DROP INDEX "idx_drop_index_users_name"`; queries[0] != want {
		t.Errorf("DropIndex SQL mismatch:\n got: %s\nwant: %s", queries[0], want)
	}
	if want := `ALTER INDEX "idx_old" RENAME TO "idx_new"`; queries[1] != want {
		t.Errorf("RenameIndex SQL mismatch:\n got: %s\nwant: %s", queries[1], want)
	}
}

// With an explicit schema-qualified table the index must be qualified with a
// quoted identifier (a bare string would be sent as a bind parameter, which
// is also invalid in an identifier position).
func TestDropAndRenameIndexWithExplicitSchema(t *testing.T) {
	pool := &recordingPool{}
	db := openWithPool(t, pool)

	if err := db.Migrator().DropIndex(&schemaQualifiedUser{}, "idx_drop_index_users_name"); err != nil {
		t.Fatalf("DropIndex: %v", err)
	}
	if err := db.Migrator().RenameIndex(&schemaQualifiedUser{}, "idx_old", "idx_new"); err != nil {
		t.Fatalf("RenameIndex: %v", err)
	}

	queries := pool.recorded()
	if len(queries) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(queries), queries)
	}
	if want := `DROP INDEX "public"."idx_drop_index_users_name"`; queries[0] != want {
		t.Errorf("DropIndex SQL mismatch:\n got: %s\nwant: %s", queries[0], want)
	}
	if want := `ALTER INDEX "public"."idx_old" RENAME TO "idx_new"`; queries[1] != want {
		t.Errorf("RenameIndex SQL mismatch:\n got: %s\nwant: %s", queries[1], want)
	}
}
