package instance_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSessionQueryDatabaseContext(t *testing.T) {
	for _, operation := range []string{"get", "proxy"} {
		t.Run(operation, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			r := &instanceRepository{db: gormDB}
			id := "11111111-1111-4111-8111-111111111111"
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if operation == "get" {
				mock.ExpectQuery("SELECT .* FROM .*instances.*").WillDelayFor(time.Second).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
				_, err = r.GetInstanceByIDContext(ctx, id)
			} else {
				mock.ExpectExec("UPDATE .*instances.*").WillDelayFor(time.Second).WillReturnResult(sqlmock.NewResult(0, 1))
				err = r.UpdateProxyContext(ctx, id, "{}")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("database deadline cause = %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
