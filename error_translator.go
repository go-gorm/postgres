package postgres

import (
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var errDesToGormErrs = map[string]error{
	"23505": gorm.ErrDuplicatedKey,
	"23503": gorm.ErrForeignKeyViolated,
}

func (dialector Dialector) Translate(err error) error {
	if pgErr, ok := err.(*pgconn.PgError); ok {
		gormErr, ok := errDesToGormErrs[pgErr.Code]
		if ok {
			return gormErr
		}
	}

	return err
}
