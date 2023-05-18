package postgres

import (
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var pgerrodesToGormErrors = map[string]error{
	"23505": gorm.ErrDuplicatedKey,
	"23503": gorm.ErrForeignKeyViolated,
}

func (dialector Dialector) Translate(err error) error {
	if pgErr, ok := err.(*pgconn.PgError); ok {
		val, ok := pgerrodesToGormErrors[pgErr.Code]
		if ok {
			return val
		}
	}

	return err
}
