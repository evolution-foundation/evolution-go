package instance_repository

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const persistenceInstanceID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func settingsRepositoryForTest(t *testing.T) (*instanceRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mock.ExpectClose()
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DisableAutomaticPing: true, Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &instanceRepository{db: db}, mock
}

func TestSettingsPersistenceScopesUpdatesAndPropagatesFailures(t *testing.T) {
	for _, kind := range []string{"advanced", "connect"} {
		for _, outcome := range []string{"success", "no_rows", "database_error"} {
			t.Run(kind+"/"+outcome, func(t *testing.T) {
				repo, mock := settingsRepositoryForTest(t)
				mock.ExpectBegin()
				var exec *sqlmock.ExpectedExec
				message := ""
				if kind == "advanced" {
					exec = mock.ExpectExec("^"+regexp.QuoteMeta(`UPDATE "instances" SET "always_online"=$1,"msg_reject_call"=$2 WHERE id = $3`)+"$").
						WithArgs(false, message, persistenceInstanceID)
				} else {
					exec = mock.ExpectExec("^"+regexp.QuoteMeta(`UPDATE "instances" SET "rabbitmq_enable"=$1 WHERE id = $2`)+"$").
						WithArgs("disabled", persistenceInstanceID)
				}
				failure := errors.New("database update failed")
				if outcome == "database_error" {
					exec.WillReturnError(failure)
					mock.ExpectRollback()
				} else {
					rows := int64(1)
					if outcome == "no_rows" {
						rows = 0
					}
					exec.WillReturnResult(sqlmock.NewResult(0, rows))
					mock.ExpectCommit()
				}
				var err error
				if kind == "advanced" {
					err = repo.UpdateAdvancedSettings(persistenceInstanceID, &instance_model.AdvancedSettings{
						AlwaysOnline: instance_model.BoolPtr(false), MsgRejectCall: &message,
					})
				} else {
					err = repo.UpdateConnectSettings(persistenceInstanceID, map[string]interface{}{"rabbitmq_enable": "disabled"})
				}
				switch outcome {
				case "success":
					if err != nil {
						t.Fatal(err)
					}
				case "no_rows":
					if !errors.Is(err, instance_model.ErrInstanceNotFound) {
						t.Fatalf("missing instance: %v", err)
					}
				case "database_error":
					if !errors.Is(err, failure) {
						t.Fatalf("lost database error: %v", err)
					}
				}
			})
		}
	}
}

func TestSettingsPersistenceNoOp(t *testing.T) {
	repo, _ := settingsRepositoryForTest(t)
	if err := repo.UpdateConnectSettings(persistenceInstanceID, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateAdvancedSettings(persistenceInstanceID, &instance_model.AdvancedSettings{}); err != nil {
		t.Fatal(err)
	}
}

func TestGetAdvancedSettingsReturnsFullPersistedValues(t *testing.T) {
	for _, outcome := range []string{"success", "not_found", "database_error"} {
		t.Run(outcome, func(t *testing.T) {
			repo, mock := settingsRepositoryForTest(t)
			query := mock.ExpectQuery(`SELECT .* FROM "instances" WHERE id = \$1 ORDER BY "instances"."id" LIMIT \$2`).
				WithArgs(persistenceInstanceID, 1)
			failure := errors.New("database read failed")
			if outcome == "database_error" {
				query.WillReturnError(failure)
			} else {
				rows := sqlmock.NewRows([]string{"always_online", "reject_call", "msg_reject_call", "read_messages", "ignore_groups", "ignore_status"})
				if outcome == "success" {
					rows.AddRow(true, false, "", false, true, false)
				}
				query.WillReturnRows(rows)
			}
			settings, err := repo.GetAdvancedSettings(persistenceInstanceID)
			if outcome == "not_found" {
				if !errors.Is(err, instance_model.ErrInstanceNotFound) || settings != nil {
					t.Fatalf("missing instance: %v", err)
				}
			} else if outcome == "database_error" {
				if !errors.Is(err, failure) || settings != nil {
					t.Fatalf("lost read error: %v", err)
				}
			} else if err != nil || settings == nil || settings.AlwaysOnline == nil || !*settings.AlwaysOnline ||
				settings.RejectCall == nil || *settings.RejectCall || settings.MsgRejectCall == nil || *settings.MsgRejectCall != "" ||
				settings.ReadMessages == nil || *settings.ReadMessages || settings.IgnoreGroups == nil || !*settings.IgnoreGroups ||
				settings.IgnoreStatus == nil || *settings.IgnoreStatus {
				t.Fatalf("incomplete settings: %#v, err=%v", settings, err)
			}
		})
	}
}
