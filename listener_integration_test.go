package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// integrationDB opens a *gorm.DB against the PostgreSQL server configured via
// the POSTGRES_DSN environment variable, skipping the test when it is unset.
func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is not set, skipping integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newTestListener(t *testing.T, ctx context.Context, db *gorm.DB) *postgres.Listener {
	t.Helper()
	listener, err := postgres.NewListener(ctx, db)
	if err != nil {
		t.Fatalf("NewListener() error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func notify(t *testing.T, ctx context.Context, db *gorm.DB, channel, payload string) {
	t.Helper()
	if err := db.WithContext(ctx).Exec("SELECT pg_notify(?, ?)", channel, payload).Error; err != nil {
		t.Fatalf("pg_notify(%q, %q) error = %v", channel, payload, err)
	}
}

func waitForNotification(t *testing.T, listener *postgres.Listener) *postgres.Notification {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n, err := listener.WaitForNotification(ctx)
	if err != nil {
		t.Fatalf("WaitForNotification() error = %v", err)
	}
	return n
}

func TestListener_receiveNotification(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	listener := newTestListener(t, ctx, db)

	if err := listener.Listen(ctx, "gorm_test_events"); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	// Send the notification from a dedicated connection so the notifying
	// backend's PID is known.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	senderConn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatalf("sqlDB.Conn() error = %v", err)
	}
	defer senderConn.Close()
	var senderPID uint32
	if err := senderConn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&senderPID); err != nil {
		t.Fatalf("pg_backend_pid() error = %v", err)
	}
	if _, err := senderConn.ExecContext(ctx, "SELECT pg_notify($1, $2)", "gorm_test_events", "hello"); err != nil {
		t.Fatalf("pg_notify error = %v", err)
	}

	n := waitForNotification(t, listener)
	if n.Channel != "gorm_test_events" {
		t.Errorf("Channel = %q, want %q", n.Channel, "gorm_test_events")
	}
	if n.Payload != "hello" {
		t.Errorf("Payload = %q, want %q", n.Payload, "hello")
	}
	if n.PID != senderPID {
		t.Errorf("PID = %d, want sender backend PID %d", n.PID, senderPID)
	}
}

func TestListener_multipleChannels(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	listener := newTestListener(t, ctx, db)

	for _, channel := range []string{"gorm_test_a", "gorm_test_b"} {
		if err := listener.Listen(ctx, channel); err != nil {
			t.Fatalf("Listen(%q) error = %v", channel, err)
		}
	}

	notify(t, ctx, db, "gorm_test_a", "1")
	notify(t, ctx, db, "gorm_test_b", "2")

	first := waitForNotification(t, listener)
	second := waitForNotification(t, listener)
	if first.Channel != "gorm_test_a" || first.Payload != "1" {
		t.Errorf("first notification = %+v, want channel gorm_test_a payload 1", first)
	}
	if second.Channel != "gorm_test_b" || second.Payload != "2" {
		t.Errorf("second notification = %+v, want channel gorm_test_b payload 2", second)
	}
}

// TestListener_unlisten verifies that notifications sent after Unlisten are
// not delivered. A control channel avoids timing-based assertions: PostgreSQL
// delivers notifications from different transactions in commit order, so if
// the control notification arrives first, the unlistened one was dropped.
func TestListener_unlisten(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	listener := newTestListener(t, ctx, db)

	for _, channel := range []string{"gorm_test_dropped", "gorm_test_control"} {
		if err := listener.Listen(ctx, channel); err != nil {
			t.Fatalf("Listen(%q) error = %v", channel, err)
		}
	}
	if err := listener.Unlisten(ctx, "gorm_test_dropped"); err != nil {
		t.Fatalf("Unlisten() error = %v", err)
	}

	notify(t, ctx, db, "gorm_test_dropped", "should not arrive")
	notify(t, ctx, db, "gorm_test_control", "control")

	n := waitForNotification(t, listener)
	if n.Channel != "gorm_test_control" {
		t.Errorf("received notification on %q, want only %q", n.Channel, "gorm_test_control")
	}
}

func TestListener_unlistenAll(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	listener := newTestListener(t, ctx, db)

	for _, channel := range []string{"gorm_test_one", "gorm_test_two"} {
		if err := listener.Listen(ctx, channel); err != nil {
			t.Fatalf("Listen(%q) error = %v", channel, err)
		}
	}
	if err := listener.UnlistenAll(ctx); err != nil {
		t.Fatalf("UnlistenAll() error = %v", err)
	}
	// Re-register only the control channel; notifications for the previously
	// registered channels must no longer be delivered.
	if err := listener.Listen(ctx, "gorm_test_control"); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	notify(t, ctx, db, "gorm_test_one", "1")
	notify(t, ctx, db, "gorm_test_two", "2")
	notify(t, ctx, db, "gorm_test_control", "control")

	n := waitForNotification(t, listener)
	if n.Channel != "gorm_test_control" {
		t.Errorf("received notification on %q, want only %q", n.Channel, "gorm_test_control")
	}
}

func TestListener_unusualChannelNames(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	for _, channel := range []string{"my channel", "my-channel", "MyChannel", `we"ird`} {
		t.Run(channel, func(t *testing.T) {
			listener := newTestListener(t, ctx, db)
			if err := listener.Listen(ctx, channel); err != nil {
				t.Fatalf("Listen(%q) error = %v", channel, err)
			}
			notify(t, ctx, db, channel, "payload")
			n := waitForNotification(t, listener)
			if n.Channel != channel {
				t.Errorf("Channel = %q, want %q", n.Channel, channel)
			}
		})
	}
}

func TestListener_emptyChannel(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	listener := newTestListener(t, ctx, db)

	if err := listener.Listen(ctx, ""); err == nil {
		t.Error("Listen(\"\") succeeded, want error")
	}
	if err := listener.Unlisten(ctx, ""); err == nil {
		t.Error("Unlisten(\"\") succeeded, want error")
	}
}

func TestListener_waitCanceled(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	listener := newTestListener(t, ctx, db)

	if err := listener.Listen(ctx, "gorm_test_cancel"); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	waitCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := listener.WaitForNotification(waitCtx)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitForNotification() error = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WaitForNotification() did not return after context cancellation")
	}

	// A canceled wait must leave the connection and its registrations usable.
	notify(t, ctx, db, "gorm_test_cancel", "after cancel")
	n := waitForNotification(t, listener)
	if n.Payload != "after cancel" {
		t.Errorf("Payload = %q, want %q", n.Payload, "after cancel")
	}

	// A deadline-exceeded wait reports context.DeadlineExceeded.
	timeoutCtx, cancelTimeout := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancelTimeout()
	if _, err := listener.WaitForNotification(timeoutCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForNotification() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestListener_transactionCommitAndRollback(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	listener := newTestListener(t, ctx, db)

	if err := listener.Listen(ctx, "gorm_test_tx"); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	// A notification sent inside a rolled-back transaction is never
	// delivered.
	rollbackErr := errors.New("force rollback")
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		notify(t, ctx, tx, "gorm_test_tx", "rolled back")
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("Transaction() error = %v, want forced rollback", err)
	}

	// A notification sent inside a committed transaction is delivered only
	// after commit: before the commit it must not be observable.
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("Begin() error = %v", tx.Error)
	}
	notify(t, ctx, tx, "gorm_test_tx", "committed")

	preCommitCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	_, waitErr := listener.WaitForNotification(preCommitCtx)
	cancel()
	if !errors.Is(waitErr, context.DeadlineExceeded) {
		t.Fatalf("WaitForNotification() before commit error = %v, want context.DeadlineExceeded", waitErr)
	}

	if err := tx.Commit().Error; err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	n := waitForNotification(t, listener)
	if n.Payload != "committed" {
		t.Errorf("Payload = %q, want %q (rolled-back notification must not be delivered)", n.Payload, "committed")
	}
}

func TestListener_unrelatedChannels(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	listener := newTestListener(t, ctx, db)

	if err := listener.Listen(ctx, "gorm_test_mine"); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	notify(t, ctx, db, "gorm_test_other", "not mine")
	notify(t, ctx, db, "gorm_test_mine", "mine")

	n := waitForNotification(t, listener)
	if n.Channel != "gorm_test_mine" || n.Payload != "mine" {
		t.Errorf("notification = %+v, want only the gorm_test_mine notification", n)
	}
}

func TestListener_closeReleasesConnection(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.SetMaxOpenConns(0) })

	listener, err := postgres.NewListener(ctx, db)
	if err != nil {
		t.Fatalf("NewListener() error = %v", err)
	}

	// While the listener holds the pool's only connection, other queries
	// cannot run.
	busyCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	busyErr := db.WithContext(busyCtx).Exec("SELECT 1").Error
	cancel()
	if busyErr == nil {
		t.Fatal("Exec() succeeded while the listener held the pool's only connection, want timeout")
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// After Close the pool slot is free again.
	if err := db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		t.Fatalf("Exec() after Close error = %v", err)
	}
}

func TestListener_afterClose(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	listener, err := postgres.NewListener(ctx, db)
	if err != nil {
		t.Fatalf("NewListener() error = %v", err)
	}
	if err := listener.Listen(ctx, "gorm_test_closed"); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}

	if err := listener.Listen(ctx, "gorm_test_closed"); !errors.Is(err, postgres.ErrListenerClosed) {
		t.Errorf("Listen() after Close error = %v, want ErrListenerClosed", err)
	}
	if err := listener.Unlisten(ctx, "gorm_test_closed"); !errors.Is(err, postgres.ErrListenerClosed) {
		t.Errorf("Unlisten() after Close error = %v, want ErrListenerClosed", err)
	}
	if err := listener.UnlistenAll(ctx); !errors.Is(err, postgres.ErrListenerClosed) {
		t.Errorf("UnlistenAll() after Close error = %v, want ErrListenerClosed", err)
	}
	if _, err := listener.WaitForNotification(ctx); !errors.Is(err, postgres.ErrListenerClosed) {
		t.Errorf("WaitForNotification() after Close error = %v, want ErrListenerClosed", err)
	}
}

func TestListener_sessionAndTransactionHandles(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()

	t.Run("session handle", func(t *testing.T) {
		session := db.Session(&gorm.Session{})
		listener := newTestListener(t, ctx, session)
		if err := listener.Listen(ctx, "gorm_test_session"); err != nil {
			t.Fatalf("Listen() error = %v", err)
		}
		notify(t, ctx, db, "gorm_test_session", "via session")
		if n := waitForNotification(t, listener); n.Payload != "via session" {
			t.Errorf("Payload = %q, want %q", n.Payload, "via session")
		}
	})

	// A transaction-derived handle yields an independent listener connection;
	// the listener does not join the transaction.
	t.Run("transaction handle", func(t *testing.T) {
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			listener, err := postgres.NewListener(ctx, tx)
			if err != nil {
				return err
			}
			defer listener.Close()
			if err := listener.Listen(ctx, "gorm_test_tx_handle"); err != nil {
				return err
			}
			// Sent outside the transaction, so delivered immediately.
			notify(t, ctx, db, "gorm_test_tx_handle", "independent")
			n := waitForNotification(t, listener)
			if n.Payload != "independent" {
				t.Errorf("Payload = %q, want %q", n.Payload, "independent")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Transaction() error = %v", err)
		}
	})
}
