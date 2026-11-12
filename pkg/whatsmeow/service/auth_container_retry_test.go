package whatsmeow_service

import (
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/evolution-foundation/evolution-go/pkg/config"
)

func TestGetAuthContainerRetryAndReuse(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "does-not-exist")
	goodPath := t.TempDir()

	// Both attempts share the same per-service auth store.
	authStore := &authStoreState{}

	svc := whatsmeowService{
		config:    &config.Config{},
		exPath:    badPath,
		authStore: authStore,
	}

	t.Cleanup(func() {
		authStore.mu.Lock()
		defer authStore.mu.Unlock()

		if authStore.container != nil {
			_ = authStore.container.Close()
			authStore.container = nil
		}
	})

	// SQLite cannot create the missing dbdata directory.
	if _, err := svc.getAuthContainer(); err == nil {
		t.Fatal("expected an error for a missing directory, got nil")
	}

	authStore.mu.Lock()
	if authStore.container != nil {
		authStore.mu.Unlock()
		t.Fatal("failed container should not be memoized")
	}
	authStore.mu.Unlock()

	// Prepare the directory and retry using the same service state.
	if err := os.MkdirAll(filepath.Join(goodPath, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}

	svc.exPath = goodPath

	c1, err := svc.getAuthContainer()
	if err != nil {
		t.Fatalf("retry after failure should succeed: %v", err)
	}
	if c1 == nil {
		t.Fatal("nil container after success")
	}

	c2, err := svc.getAuthContainer()
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if c1 != c2 {
		t.Fatal("container was not reused")
	}
}
