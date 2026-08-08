package postgres_test

import (
	"context"
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ExampleNewListener demonstrates receiving a PostgreSQL notification. The
// listener reserves one dedicated connection until Close is called, and its
// LISTEN registrations last only for that connection's session.
func ExampleNewListener() {
	dsn := "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	listener, err := postgres.NewListener(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	if err := listener.Listen(ctx, "events"); err != nil {
		log.Fatal(err)
	}

	// Notifications can be published through GORM. Inside a transaction,
	// PostgreSQL delivers them only after the transaction commits.
	if err := db.WithContext(ctx).Exec("SELECT pg_notify(?, ?)", "events", "hello").Error; err != nil {
		log.Fatal(err)
	}

	// WaitForNotification blocks until a notification arrives or ctx is
	// done; cancel the context to stop waiting.
	notification, err := listener.WaitForNotification(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(notification.Channel, notification.Payload)
}
