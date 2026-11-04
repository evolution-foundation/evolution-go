package whatsmeow_service

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/store/sqlstore/upgrades"
	_ "modernc.org/sqlite"

	"github.com/evolution-foundation/evolution-go/pkg/config"
)

func expectCurrentPostgresAuthSchema(mock sqlmock.Sqlmock) {
	columnExists := regexp.QuoteMeta("SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name=$1 AND column_name=$2)")
	mock.ExpectQuery(columnExists).WithArgs("whatsmeow_version", "compat").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	version := len(upgrades.Table)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT version, compat FROM whatsmeow_version LIMIT 1")).
		WillReturnRows(sqlmock.NewRows([]string{"version", "compat"}).AddRow(version, version))
}

func postgresAuthService(t *testing.T) (whatsmeowService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	return whatsmeowService{
		config:    &config.Config{PostgresAuthDB: "postgres://existing-pool"},
		authDB:    db,
		authStore: &authStoreState{},
	}, mock
}

func TestPostgresAuthContainerReusesExistingBoundedPool(t *testing.T) {
	w, mock := postgresAuthService(t)
	expectCurrentPostgresAuthSchema(mock)
	first, err := w.getAuthContainer()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		container, err := w.getAuthContainer()
		if err != nil || container != first {
			t.Fatalf("reconnect %d did not reuse the auth store: %v", i, err)
		}
	}
	if got := w.authDB.Stats().MaxOpenConnections; got != 25 {
		t.Fatalf("shared auth pool limit changed to %d", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("auth schema was queried again during reconnect: %v", err)
	}
}

func TestPostgresAuthContainerRetriesAfterTooManyClients(t *testing.T) {
	w, mock := postgresAuthService(t)
	columnExists := regexp.QuoteMeta("SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name=$1 AND column_name=$2)")
	mock.ExpectQuery(columnExists).WithArgs("whatsmeow_version", "compat").
		WillReturnError(errors.New("pq: sorry, too many clients already"))
	if _, err := w.getAuthContainer(); err == nil || !regexp.MustCompile("too many clients already").MatchString(err.Error()) {
		t.Fatalf("expected production connection error, got %v", err)
	}
	if err := w.authDB.Ping(); err != nil {
		t.Fatalf("failed upgrade closed the process-owned authDB: %v", err)
	}
	expectCurrentPostgresAuthSchema(mock)
	if _, err := w.getAuthContainer(); err != nil {
		t.Fatalf("auth store did not recover after DB availability returned: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func sqliteAuthService(t *testing.T) whatsmeowService {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dbdata"), 0700); err != nil {
		t.Fatal(err)
	}
	return whatsmeowService{
		config:    &config.Config{},
		exPath:    filepath.ToSlash(dir),
		authStore: &authStoreState{},
	}
}

func TestAuthContainerReusedAcrossClientStarts(t *testing.T) {
	w := sqliteAuthService(t)
	first, err := w.getAuthContainer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })

	for i := 0; i < 10; i++ {
		container, err := w.getAuthContainer()
		if err != nil {
			t.Fatalf("client start %d: %v", i, err)
		}
		if container != first {
			_ = container.Close()
			t.Fatalf("client start %d opened another auth store", i)
		}
	}
}

func TestAuthContainerConcurrentStartsShareOneStore(t *testing.T) {
	w := sqliteAuthService(t)
	const starts = 12
	containers := make([]*sqlstore.Container, starts)
	errs := make([]error, starts)
	var wg sync.WaitGroup
	for i := range containers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			containers[i], errs[i] = w.getAuthContainer()
		}(i)
	}
	wg.Wait()
	for i, container := range containers {
		if errs[i] != nil {
			t.Fatalf("concurrent client start %d: %v", i, errs[i])
		}
		if container != containers[0] {
			t.Fatalf("concurrent client start %d opened another auth store", i)
		}
	}
	t.Cleanup(func() { _ = containers[0].Close() })
}

func TestAuthContainerRetriesAfterInitialFailure(t *testing.T) {
	dir := t.TempDir()
	w := whatsmeowService{
		config:    &config.Config{},
		exPath:    filepath.ToSlash(dir),
		authStore: &authStoreState{},
	}
	if _, err := w.getAuthContainer(); err == nil {
		t.Fatal("expected missing dbdata directory to fail")
	}
	if err := os.MkdirAll(filepath.Join(dir, "dbdata"), 0700); err != nil {
		t.Fatal(err)
	}
	container, err := w.getAuthContainer()
	if err != nil {
		t.Fatalf("auth store did not recover after directory appeared: %v", err)
	}
	t.Cleanup(func() { _ = container.Close() })
}
