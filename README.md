# GORM PostgreSQL Driver

## USAGE

```go
import (
  "gorm.io/driver/postgres"
  "gorm.io/gorm"
)

// https://github.com/lib/pq
dsn := "user=gorm password=gorm DB.name=gorm port=9920 sslmode=disable TimeZone=Asia/Shanghai"
db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
```

Checkout [https://gorm.io](https://gorm.io) for details.
