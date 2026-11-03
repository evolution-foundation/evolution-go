package whatsmeow_service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	_ "modernc.org/sqlite"
)

func TestGetContainerIsSharedAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := whatsmeowService{config: &config.Config{}, exPath: dir, deviceContainer: &sharedContainer{}}
	t.Cleanup(func() {
		if w.deviceContainer.container != nil {
			w.deviceContainer.container.Close()
		}
	})

	const callers = 8
	got := make([]any, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := w.getContainer()
			if err != nil {
				t.Error(err)
				return
			}
			got[i] = c
		}()
	}
	wg.Wait()

	for i := 1; i < callers; i++ {
		if got[i] != got[0] {
			t.Fatalf("call %d returned a different container", i)
		}
	}

	// O container continua utilizável: nenhuma instância deve fechá-lo.
	c, _ := w.getContainer()
	if _, err := c.GetAllDevices(context.Background()); err != nil {
		t.Fatalf("shared container unusable: %v", err)
	}
}

func TestGetContainerRequiresAuthDBWithPostgres(t *testing.T) {
	w := whatsmeowService{config: &config.Config{PostgresAuthDB: "postgres://unused"}, deviceContainer: &sharedContainer{}}
	if _, err := w.getContainer(); err == nil {
		t.Fatal("expected error when authDB is nil")
	}
	if w.deviceContainer.container != nil {
		t.Fatal("failed creation must not be cached")
	}
}
