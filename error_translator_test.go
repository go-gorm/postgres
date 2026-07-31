package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestDialector_Translate(t *testing.T) {
	type fields struct {
		Config *Config
	}
	type args struct {
		err error
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   error
	}{
		{
			name: "it should return ErrDuplicatedKey error if the status code is 23505",
			args: args{err: &pgconn.PgError{Code: "23505"}},
			want: gorm.ErrDuplicatedKey,
		},
		{
			name: "it should return ErrForeignKeyViolated error if the status code is 23503",
			args: args{err: &pgconn.PgError{Code: "23503"}},
			want: gorm.ErrForeignKeyViolated,
		},
		{
			name: "it should return gorm.ErrInvalidField error if the status code is 42703",
			args: args{err: &pgconn.PgError{Code: "42703"}},
			want: gorm.ErrInvalidField,
		},
		{
			name: "it should return gorm.ErrCheckConstraintViolated error if the status code is 23514",
			args: args{err: &pgconn.PgError{Code: "23514"}},
			want: gorm.ErrCheckConstraintViolated,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialector := Dialector{
				Config: tt.fields.Config,
			}
			if err := dialector.Translate(tt.args.err); !errors.Is(err, tt.want) {
				t.Errorf("Translate() expected error = %v, got error %v", err, tt.want)
			}
		})
	}
}

func TestDialector_Translate_PreservesOriginalError(t *testing.T) {
	tests := []struct {
		name        string
		pgErr       *pgconn.PgError
		wantGormErr error
	}{
		{
			name: "it should preserve original pgError detail on ErrDuplicatedKey",
			pgErr: &pgconn.PgError{
				Code:           "23505",
				Message:        "duplicate key value violates unique constraint",
				Detail:         "Key (email)=(foo@bar.com) already exists.",
				ConstraintName: "users_email_key",
			},
			wantGormErr: gorm.ErrDuplicatedKey,
		},
		{
			name: "it should preserve original pgError detail on ErrForeignKeyViolated",
			pgErr: &pgconn.PgError{
				Code:           "23503",
				Message:        "insert or update on table violates foreign key constraint",
				Detail:         "Key (user_id)=(999) is not present in table \"users\".",
				ConstraintName: "orders_user_id_fkey",
			},
			wantGormErr: gorm.ErrForeignKeyViolated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialector := Dialector{}
			err := dialector.Translate(tt.pgErr)

			// errors.Is() must still work after wrapping
			if !errors.Is(err, tt.wantGormErr) {
				t.Errorf("errors.Is() expected %v, got %v", tt.wantGormErr, err)
			}

			// errors.As() must be able to unwrap original pgErr
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Errorf("errors.As() failed: original *pgconn.PgError is not accessible")
				return
			}

			// original detail must be preserved
			if pgErr.Detail != tt.pgErr.Detail {
				t.Errorf("Detail expected %q, got %q", tt.pgErr.Detail, pgErr.Detail)
			}
			if pgErr.ConstraintName != tt.pgErr.ConstraintName {
				t.Errorf("ConstraintName expected %q, got %q", tt.pgErr.ConstraintName, pgErr.ConstraintName)
			}
		})
	}
}