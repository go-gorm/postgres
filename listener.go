package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/gorm"
)

var (
	// ErrUnsupportedConnection is returned by NewListener when the underlying
	// database connection is not managed by the pgx stdlib driver, for example
	// when a custom Config.Conn or Config.DriverName routes connections
	// through a different database/sql driver.
	ErrUnsupportedConnection = errors.New("postgres: LISTEN requires a pgx stdlib connection")

	// ErrListenerClosed is returned by all Listener methods after Close has
	// been called.
	ErrListenerClosed = errors.New("postgres: listener is closed")
)

// Notification is a PostgreSQL notification received by a Listener.
type Notification struct {
	// PID is the backend process ID of the PostgreSQL session that sent the
	// notification.
	PID uint32
	// Channel is the name of the channel the notification was sent on.
	Channel string
	// Payload is the optional payload string sent with the notification.
	Payload string
}

// Listener receives PostgreSQL NOTIFY messages using LISTEN.
//
// A Listener reserves one dedicated connection from the *sql.DB behind the
// *gorm.DB for its entire lifetime, so that connection is unavailable to
// other queries until Close is called. A pool limited to a single connection
// (for example via SetMaxOpenConns(1)) cannot run other queries, such as
// sending notifications, while a Listener exists.
//
// LISTEN registrations are scoped to the reserved PostgreSQL session: they
// disappear when the Listener is closed or the connection is lost, and
// notifications sent while no session is listening are dropped. PostgreSQL
// notifications are not a persistent queue; a Listener may miss notifications
// while it is disconnected. Reconnecting and re-subscribing after a
// connection loss is the application's responsibility - the Listener never
// reconnects on its own.
//
// Notifications can be published through GORM with
//
//	db.Exec("SELECT pg_notify(?, ?)", channel, payload)
//
// When pg_notify (or NOTIFY) runs inside a transaction, PostgreSQL delivers
// the notification only after the transaction commits, and never if it rolls
// back.
//
// A Listener is not safe for concurrent use. Calls must be serialized by the
// caller, and Listen, Unlisten, UnlistenAll and WaitForNotification must not
// run concurrently with each other or with Close. To interrupt a blocked
// WaitForNotification, cancel the context passed to it; afterwards the
// Listener remains usable and may be closed or reused.
type Listener struct {
	conn   *sql.Conn
	closed atomic.Bool
}

// NewListener reserves a dedicated connection from the connection pool behind
// db and returns a Listener bound to that connection's PostgreSQL session.
//
// db may be a root, session or transaction *gorm.DB handle; in every case the
// Listener acquires its own independent connection from the pool and never
// joins an ongoing transaction. The connection pool must be backed by the pgx
// stdlib driver (the default for this dialector); otherwise NewListener
// returns an error wrapping ErrUnsupportedConnection.
//
// The caller must call Close to release the reserved connection.
func NewListener(ctx context.Context, db *gorm.DB) (*Listener, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres: resolve *sql.DB for listener: %w", err)
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: acquire listener connection: %w", err)
	}

	supported := false
	if err := conn.Raw(func(driverConn any) error {
		_, supported = driverConn.(*stdlib.Conn)
		return nil
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("postgres: inspect listener connection: %w", err)
	}
	if !supported {
		_ = conn.Close()
		return nil, ErrUnsupportedConnection
	}

	return &Listener{conn: conn}, nil
}

// Listen registers the Listener's session as a listener on channel, which is
// quoted as a SQL identifier and therefore matched case-sensitively.
func (l *Listener) Listen(ctx context.Context, channel string) error {
	quoted, err := quoteChannel(channel)
	if err != nil {
		return err
	}
	return l.exec(ctx, "LISTEN "+quoted)
}

// Unlisten removes the session's registration on channel. Unlistening a
// channel that is not registered is not an error.
func (l *Listener) Unlisten(ctx context.Context, channel string) error {
	quoted, err := quoteChannel(channel)
	if err != nil {
		return err
	}
	return l.exec(ctx, "UNLISTEN "+quoted)
}

// UnlistenAll removes all of the session's channel registrations using
// UNLISTEN *.
func (l *Listener) UnlistenAll(ctx context.Context) error {
	return l.exec(ctx, "UNLISTEN *")
}

// WaitForNotification blocks until a notification is received on one of the
// registered channels or ctx is done.
//
// When ctx is canceled or times out, the returned error satisfies
// errors.Is(err, context.Canceled) or errors.Is(err, context.DeadlineExceeded)
// and the Listener remains usable. Any other error usually means the
// underlying connection is broken; the Listener should then be closed and, if
// desired, replaced with a new one.
func (l *Listener) WaitForNotification(ctx context.Context) (*Notification, error) {
	var notification *Notification
	err := l.raw(func(conn *pgx.Conn) error {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		notification = &Notification{PID: n.PID, Channel: n.Channel, Payload: n.Payload}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return notification, nil
}

// Close releases the reserved connection. Because the session may still hold
// LISTEN registrations and buffered notifications, the physical connection is
// discarded instead of being reused; its slot in the pool is freed either
// way. Close is idempotent: the first call releases the connection and
// subsequent calls return nil. All other methods return ErrListenerClosed
// after Close.
func (l *Listener) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Returning driver.ErrBadConn makes database/sql discard the underlying
	// driver connection rather than returning the session to the pool.
	_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
	if err := l.conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
		return fmt.Errorf("postgres: close listener connection: %w", err)
	}
	return nil
}

// raw runs f against the reserved pgx connection.
func (l *Listener) raw(f func(conn *pgx.Conn) error) error {
	if l.closed.Load() {
		return ErrListenerClosed
	}
	return l.conn.Raw(func(driverConn any) error {
		stdConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return ErrUnsupportedConnection
		}
		return f(stdConn.Conn())
	})
}

func (l *Listener) exec(ctx context.Context, sql string) error {
	return l.raw(func(conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, sql); err != nil {
			return fmt.Errorf("postgres: %s: %w", sql, err)
		}
		return nil
	})
}

// quoteChannel quotes a notification channel name as a SQL identifier.
// Channel names are identifiers, not bind values, so they cannot be passed as
// ordinary query parameters.
func quoteChannel(channel string) (string, error) {
	if channel == "" {
		return "", errors.New("postgres: notification channel name must not be empty")
	}
	return pgx.Identifier{channel}.Sanitize(), nil
}
