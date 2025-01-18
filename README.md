# GORM PostgreSQL Driver

## Quick Start

```go
import (
  "gorm.io/driver/postgres"
  "gorm.io/gorm"
)

// https://github.com/jackc/pgx
dsn := "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable TimeZone=Asia/Shanghai"
db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
```

## Configuration

```go
import (
  "gorm.io/driver/postgres"
  "gorm.io/gorm"
)

db, err := gorm.Open(postgres.New(postgres.Config{
  DSN: "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable TimeZone=Asia/Shanghai", // data source name, refer https://github.com/jackc/pgx
  PreferSimpleProtocol: true, // disables implicit prepared statement usage. By default pgx automatically uses the extended protocol
}), &gorm.Config{})
```

## Example Usage

```go
import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)
  
type Postgress struct {
	DB *gorm.DB
}

 func (store *Postgress) NewStore() error  {
	dsn := "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable TimeZone=Asia/Shanghai"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})

	if err != nil {
		fmt.Println("connection failed")
		return err
	}else{
		fmt.Println("Database connected successfully")
		store.DB = db
	}
  
	return nil
 }
 ```

Checkout [https://gorm.io](https://gorm.io) for details.
