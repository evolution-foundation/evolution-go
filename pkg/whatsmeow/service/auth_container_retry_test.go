package whatsmeow_service

import (
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/evolution-foundation/evolution-go/pkg/config"
)

// Exercised through SQLite only. Closing ownDB on the Upgrade error path is not
// covered: the handle is local to getAuthContainer.
func TestGetAuthContainerRetryAndReuse(t *testing.T) {
	// TempDirs first: cleanups run LIFO, so reset closes the *sql.DB before
	// RemoveAll, which fails on Windows while the file is still open.
	badPath := filepath.Join(t.TempDir(), "does-not-exist")
	goodPath := t.TempDir()

	reset := func() {
		sharedAuthContainerMu.Lock()
		if sharedAuthContainer != nil {
			_ = sharedAuthContainer.Close()
			sharedAuthContainer = nil
		}
		sharedAuthContainerMu.Unlock()
	}
	reset()
	t.Cleanup(reset)

	// SQLite does not create missing directories, so Upgrade fails here.
	bad := whatsmeowService{
		config: &config.Config{},
		exPath: badPath,
	}
	if _, err := bad.getAuthContainer(); err == nil {
		t.Fatal("expected an error for a missing directory, got nil")
	}
	sharedAuthContainerMu.Lock()
	if sharedAuthContainer != nil {
		sharedAuthContainerMu.Unlock()
		t.Fatalf("failed container should not be memoized (sharedAuthContainer = %#v)", sharedAuthContainer)
	}
	sharedAuthContainerMu.Unlock()

	// The DSN expects a dbdata subdirectory.
	if err := os.MkdirAll(filepath.Join(goodPath, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	good := whatsmeowService{
		config: &config.Config{},
		exPath: goodPath,
	}
	c1, err := good.getAuthContainer()
	if err != nil {
		t.Fatalf("retry after a failure should succeed instead of returning a cached error: %v", err)
	}
	if c1 == nil {
		t.Fatal("nil container after success")
	}

	c2, err := good.getAuthContainer()
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if c1 != c2 {
		t.Fatal("container was not reused; expected the shared instance")
	}
}
