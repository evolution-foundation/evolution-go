package webhook_producer

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
)

func TestWebhookLogsDoNotExposeURLQueryOrResponseBody(t *testing.T) {
	const secret = "SECRET_URL_TOKEN_DO_NOT_LOG"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("response-" + secret))
	}))
	defer server.Close()

	logDir := t.TempDir()
	producer := &webhookProducer{loggerWrapper: logger_wrapper.NewLoggerManager(&config.Config{
		LogDirectory: logDir,
	})}

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writePipe
	producer.sendWebhookWithRetry(server.URL+"/hook?token="+secret, []byte(`{"event":"test"}`), 1, 0, "instance-production-trace")
	_ = writePipe.Close()
	os.Stdout = originalStdout
	stdout, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatal(err)
	}
	_ = readPipe.Close()

	fileLog, err := os.ReadFile(filepath.Join(logDir, "instance-production-trace", "instance.log"))
	if err != nil {
		t.Fatal(err)
	}
	combined := string(stdout) + string(fileLog)
	if strings.Contains(combined, secret) || strings.Contains(combined, "?token=") {
		t.Fatalf("webhook secret leaked in logs: %s", combined)
	}
}
