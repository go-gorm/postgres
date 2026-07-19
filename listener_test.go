package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func Test_quoteChannel(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		want    string
		wantErr bool
	}{
		{name: "simple", channel: "events", want: `"events"`},
		{name: "uppercase", channel: "Events", want: `"Events"`},
		{name: "space", channel: "my channel", want: `"my channel"`},
		{name: "dash", channel: "my-channel", want: `"my-channel"`},
		{name: "embedded double quote", channel: `we"ird`, want: `"we""ird"`},
		{name: "asterisk is a plain identifier", channel: "*", want: `"*"`},
		{name: "empty", channel: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := quoteChannel(tt.channel)
			if (err != nil) != tt.wantErr {
				t.Fatalf("quoteChannel() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("quoteChannel() = %v, want %v", got, tt.want)
			}
		})
	}
}

// fakeDriver is a database/sql driver that is not pgx, used to verify that
// NewListener rejects unsupported driver connections instead of panicking.
type fakeDriver struct{}

func (fakeDriver) Open(name string) (driver.Conn, error) { return fakeConn{}, nil }

type fakeConn struct{}

func (fakeConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("fake driver: not implemented")
}
func (fakeConn) Close() error { return nil }
func (fakeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("fake driver: not implemented")
}

func openFakeGormDB(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	sqlDB := sql.OpenDB(fakeConnector{})
	db, err := gorm.Open(New(Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return db, sqlDB
}

type fakeConnector struct{}

func (fakeConnector) Connect(context.Context) (driver.Conn, error) { return fakeConn{}, nil }
func (fakeConnector) Driver() driver.Driver                        { return fakeDriver{} }

func TestNewListener_nilDB(t *testing.T) {
	if _, err := NewListener(context.Background(), nil); !errors.Is(err, gorm.ErrInvalidDB) {
		t.Fatalf("NewListener(nil) error = %v, want gorm.ErrInvalidDB", err)
	}
}

func TestNewListener_unsupportedDriver(t *testing.T) {
	db, _ := openFakeGormDB(t)
	if _, err := NewListener(context.Background(), db); !errors.Is(err, ErrUnsupportedConnection) {
		t.Fatalf("NewListener() error = %v, want ErrUnsupportedConnection", err)
	}
}

func TestNewListener_invalidConnPool(t *testing.T) {
	db, _ := openFakeGormDB(t)
	// A ConnPool that is neither *sql.DB nor a GetDBConnector cannot provide
	// a listener connection.
	db.ConnPool = fakeConnPool{}
	db.Statement.ConnPool = fakeConnPool{}
	if _, err := NewListener(context.Background(), db); !errors.Is(err, gorm.ErrInvalidDB) {
		t.Fatalf("NewListener() error = %v, want gorm.ErrInvalidDB", err)
	}
}

type fakeConnPool struct{}

func (fakeConnPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, errors.New("fake pool: not implemented")
}
func (fakeConnPool) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return nil, errors.New("fake pool: not implemented")
}
func (fakeConnPool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, errors.New("fake pool: not implemented")
}
func (fakeConnPool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return nil
}

func TestNewListener_closedDB(t *testing.T) {
	db, sqlDB := openFakeGormDB(t)
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("sqlDB.Close() error = %v", err)
	}
	if _, err := NewListener(context.Background(), db); err == nil {
		t.Fatal("NewListener() on closed database succeeded, want error")
	}
}

func TestNewListener_canceledContext(t *testing.T) {
	db, _ := openFakeGormDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewListener(ctx, db); !errors.Is(err, context.Canceled) {
		t.Fatalf("NewListener() error = %v, want context.Canceled", err)
	}
}
