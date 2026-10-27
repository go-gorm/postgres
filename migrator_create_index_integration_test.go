package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var errCatalogUnavailable = errors.New("catalog unavailable")

type failingQueryPool struct{ *sql.DB }

func (p failingQueryPool) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, errCatalogUnavailable
}

type collisionIndexModel struct {
	ID    int
	Value string `gorm:"uniqueIndex:collision_idx"`
}
type longIndexModel struct {
	ID    int
	Value string `gorm:"uniqueIndex:ééééééééééééééééééééééééééééééééééééééééééé"`
}
type concurrentIndexModel struct {
	ID    int
	Value string `gorm:"index:concurrent_idx,option:CONCURRENTLY"`
}

func TestCreateIndexPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("GORM_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("set GORM_POSTGRES_TEST_DSN to run PostgreSQL integration tests")
	}
	db, err := gorm.Open(Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	// The dedicated schema makes all database objects disposable and independent of other tests.
	schema := fmt.Sprintf("gorm_index_%d", time.Now().UnixNano())
	if err := db.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DROP SCHEMA "` + schema + `" CASCADE`) })
	table := func(name string) string { return `"` + schema + `"."` + name + `"` }
	createTable := func(name string) {
		t.Helper()
		if err := db.Exec("CREATE TABLE " + table(name) + " (id integer, value text)").Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"ordinary_target", "fresh", "Mixed Table", "long_left", "long_right", "concurrent_target"} {
		createTable(n)
	}
	// Actual migrations previously succeeded silently for a missing uniqueness constraint.
	left := db.Table(table("left"))
	right := db.Table(table("right"))
	if err := left.AutoMigrate(&collisionIndexModel{}); err != nil {
		t.Fatal(err)
	}
	if err := left.Migrator().CreateIndex(&collisionIndexModel{}, "collision_idx"); err != nil {
		t.Fatalf("same-owner repeat: %v", err)
	}
	if err := right.AutoMigrate(&collisionIndexModel{}); err == nil || !strings.Contains(err.Error(), "another relation") {
		t.Errorf("AutoMigrate collision error: %v", err)
	}
	if err := left.Create(&collisionIndexModel{ID: 1, Value: "x"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := left.Create(&collisionIndexModel{ID: 2, Value: "x"}).Error; err == nil {
		t.Error("first table did not enforce uniqueness")
	}
	if err := right.Create(&collisionIndexModel{ID: 1, Value: "x"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := right.Create(&collisionIndexModel{ID: 2, Value: "x"}).Error; err != nil {
		t.Fatalf("expected missing index to allow duplicate: %v", err)
	}
	// A table shares the index namespace and must be recognized too.
	if err := db.Exec("DROP INDEX " + table("collision_idx")).Error; err != nil {
		t.Fatal(err)
	}
	createTable("collision_idx")
	if err := db.Table(table("ordinary_target")).Migrator().CreateIndex(&collisionIndexModel{}, "collision_idx"); err == nil || !strings.Contains(err.Error(), "another relation") {
		t.Errorf("ordinary relation collision error: %v", err)
	}
	if err := db.Exec("DROP TABLE " + table("collision_idx")).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table(table("fresh")).Migrator().CreateIndex(&collisionIndexModel{}, "collision_idx"); err != nil {
		t.Fatalf("fresh index: %v", err)
	}
	// Pinned connection gives session-local search_path a stable meaning across both statements.
	err = db.Connection(func(tx *gorm.DB) error {
		if e := tx.Exec(`SET search_path TO public, "` + schema + `"`).Error; e != nil {
			return e
		}
		defer tx.Exec("RESET search_path")
		m := tx.Table(`"Mixed Table"`).Migrator()
		if e := m.CreateIndex(&collisionIndexModel{}, "collision_idx"); e == nil || !strings.Contains(e.Error(), "another relation") {
			return fmt.Errorf("search_path collision error: %v", e)
		}
		expr := tx.Table("?", clause.Table{Name: schema + ".Mixed Table"}).Migrator()
		if e := expr.CreateIndex(&collisionIndexModel{}, "collision_idx"); e == nil || !strings.Contains(e.Error(), "another relation") {
			return fmt.Errorf("expression collision error: %v", e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Table(table("Mixed Table")).Migrator().CreateIndex(&collisionIndexModel{}, "collision_idx"); e == nil || !strings.Contains(e.Error(), "another relation") {
			return fmt.Errorf("transaction collision error: %v", e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// UTF-8 identifiers are truncated by PostgreSQL at the byte boundary.
	if err := db.Table(table("long_left")).Migrator().CreateIndex(&longIndexModel{}, "Value"); err != nil {
		t.Fatal(err)
	}
	if err := db.Table(table("long_right")).Migrator().CreateIndex(&longIndexModel{}, "Value"); err == nil || !strings.Contains(err.Error(), "another relation") {
		t.Errorf("truncated UTF-8 name collision error: %v", err)
	}
	// Distinct schemas can have the same index name.
	other := schema + "_other"
	if err := db.Exec(`CREATE SCHEMA "` + other + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DROP SCHEMA "` + other + `" CASCADE`) })
	otherTable := `"` + other + `"."fresh"`
	if err := db.Exec(`CREATE TABLE ` + otherTable + ` (id integer,value text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table(otherTable).Migrator().CreateIndex(&collisionIndexModel{}, "collision_idx"); err != nil {
		t.Fatalf("separate schema: %v", err)
	}
	if err := db.Session(&gorm.Session{DryRun: true}).Table(table("Mixed Table")).AutoMigrate(&collisionIndexModel{}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	createTable("query_failure")
	failingDB, err := gorm.Open(New(Config{Conn: failingQueryPool{sqlDB}}), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := failingDB.Table(table("query_failure")).Migrator().CreateIndex(&concurrentIndexModel{}, "concurrent_idx"); !errors.Is(err, errCatalogUnavailable) {
		t.Fatalf("catalog query failure was not propagated: %v", err)
	}
	if err := db.Exec("DROP INDEX " + table("concurrent_idx")).Error; err != nil {
		t.Fatal(err)
	}
	// CONCURRENTLY cannot run in a transaction. Repeat sequentially on same target.
	c := db.Table(table("concurrent_target"))
	if err := c.Migrator().CreateIndex(&concurrentIndexModel{}, "concurrent_idx"); err != nil {
		t.Fatal(err)
	}
	if err := c.Migrator().CreateIndex(&concurrentIndexModel{}, "concurrent_idx"); err != nil {
		t.Fatalf("concurrently repeat: %v", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- c.WithContext(context.Background()).Migrator().CreateIndex(&concurrentIndexModel{}, "concurrent_idx")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			t.Errorf("concurrent repeat: %v", err)
		}
	}
}
