package whatsmeow_service

import (
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/EvolutionAPI/evolution-go/pkg/config"
)

// Valida a semântica do getAuthContainer compartilhado, usando o caminho SQLite
// (auto-contido, sem serviço externo — roda em qualquer CI):
//  1. falha na criação NÃO é memorizada (retry possível em vez de erro cacheado pra sempre)
//  2. sucesso é memorizado e o MESMO container é reusado nas chamadas seguintes
func TestGetAuthContainerRetryAndReuse(t *testing.T) {
	// estado limpo do singleton
	sharedAuthContainerMu.Lock()
	sharedAuthContainer = nil
	sharedAuthContainerMu.Unlock()

	// Falha: exPath aponta pra diretório inexistente — o SQLite não cria
	// diretórios intermediários, então o Upgrade falha na primeira conexão.
	bad := whatsmeowService{
		config: &config.Config{},
		exPath: filepath.Join(t.TempDir(), "does-not-exist"),
	}
	if _, err := bad.getAuthContainer(); err == nil {
		t.Fatal("esperava erro com diretório inexistente, veio nil")
	}
	// Falha não deve ser memorizada no singleton
	sharedAuthContainerMu.Lock()
	if sharedAuthContainer != nil {
		sharedAuthContainerMu.Unlock()
		t.Fatalf("failed container should not be memoized (sharedAuthContainer = %#v)", sharedAuthContainer)
	}
	sharedAuthContainerMu.Unlock()

	// Sucesso: exPath válido com o subdiretório dbdata esperado pelo DSN.
	goodPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(goodPath, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	good := whatsmeowService{
		config: &config.Config{},
		exPath: goodPath,
	}
	c1, err := good.getAuthContainer()
	if err != nil {
		t.Fatalf("retry após falha deveria funcionar em vez de devolver erro cacheado: %v", err)
	}
	if c1 == nil {
		t.Fatal("container nil após sucesso")
	}

	c2, err := good.getAuthContainer()
	if err != nil {
		t.Fatalf("segunda chamada falhou: %v", err)
	}
	if c1 != c2 {
		t.Fatal("container não foi reusado — deveria ser singleton compartilhado")
	}
}
