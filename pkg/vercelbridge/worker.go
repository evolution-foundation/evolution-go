// Package vercelbridge connects Vercel HTTP services to an always-on Evolution Go
// daemon through a private Supabase PostgreSQL command table.
// Added by WorkiDigital. Keep upstream licensing and authentication intact.
package vercelbridge

import (
    "bytes"
    "context"
    "database/sql"
    "errors"
    "fmt"
    "log"
    "net/http"
    "net/http/httptest"
    "time"
)

const MaxResponseBytes = 1 << 20

type command struct {
    ID string
    Method string
    Path string
    Body []byte
    EncryptedKey []byte
}

// RunWorker must run only in the continuously running Evolution Go daemon.
// Commands claimed as processing are intentionally not replayed automatically
// after crashes, because WhatsApp sends and group mutations are not idempotent.
func RunWorker(ctx context.Context, db *sql.DB, router http.Handler, tokenKeyHex string) {
    if db == nil || router == nil || ValidateTokenKey(tokenKeyHex) != nil {
        log.Printf("[vercelbridge] worker not started: missing db, router or API key")
        return
    }
    log.Printf("[vercelbridge] command worker started")
    for ctx.Err() == nil {
        c, err := claim(ctx, db)
        switch {
        case errors.Is(err, sql.ErrNoRows):
            pause(ctx, 350*time.Millisecond)
            continue
        case err != nil:
            if ctx.Err() != nil { return }
            log.Printf("[vercelbridge] claim failed: %v", err)
            pause(ctx, 3*time.Second)
            continue
        }
        process(ctx, db, router, tokenKeyHex, c)
    }
    log.Printf("[vercelbridge] command worker stopped")
}

func claim(ctx context.Context, db *sql.DB) (command, error) {
    var c command
    err := db.QueryRowContext(ctx, `
WITH next AS (
    SELECT id FROM evo_bridge.commands
    WHERE state='queued'
    ORDER BY created_at, id
    FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE evo_bridge.commands AS c
SET state='processing', started_at=now()
FROM next
WHERE c.id = next.id
RETURNING c.id::text, c.method, c.request_path, c.request_body, c.auth_ciphertext
`).Scan(&c.ID, &c.Method, &c.Path, &c.Body, &c.EncryptedKey)
    return c, err
}

func process(ctx context.Context, db *sql.DB, router http.Handler, tokenKeyHex string, c command) {
    apiKey, decryptErr := OpenAPIKey(c.EncryptedKey, tokenKeyHex)
    if decryptErr != nil {
        log.Printf("[vercelbridge] failed to decrypt credentials for command %s: %v", c.ID, decryptErr)
        finish(ctx, db, c.ID, http.StatusInternalServerError, []byte("bridge credential error"), "text/plain")
        return
    }
    // The handler is the existing authenticated Gin router in this process.
    // No inbound traffic to the daemon and no external HTTP proxy target.
    runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
    defer cancel()
    req, err := http.NewRequestWithContext(runCtx, c.Method, "http://localhost"+c.Path, bytes.NewReader(c.Body))
    if err != nil {
        finish(ctx, db, c.ID, http.StatusBadRequest, []byte("invalid request"), "text/plain")
        return
    }
    req.Header.Set("apikey", apiKey)
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    // Gin includes its own recovery middleware. If another handler panics,
    // recover and record a failure instead of killing the long-running daemon.
    code := http.StatusInternalServerError
    body := []byte("upstream request failed")
    contentType := "text/plain"
    func() {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("[vercelbridge] handler panic for command %s: %v", c.ID, r)
            }
        }()
        router.ServeHTTP(rec, req)
        result := rec.Result()
        defer result.Body.Close()
        code = result.StatusCode
        contentType = result.Header.Get("Content-Type")
        if contentType == "" { contentType = "application/json" }
        if rec.Body.Len() > MaxResponseBytes {
            code = http.StatusRequestEntityTooLarge
            body = []byte("response exceeds 1 MiB bridge limit")
            contentType = "text/plain"
            return
        }
        body = append([]byte(nil), rec.Body.Bytes()...)
    }()
    finish(ctx, db, c.ID, code, body, contentType)
}

func finish(ctx context.Context, db *sql.DB, id string, status int, body []byte, contentType string) {
    // Use a fresh bounded context even during graceful shutdown to persist
    // the result of a command that was already executed.
    commitCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
    defer cancel()
    _, err := db.ExecContext(commitCtx, `
UPDATE evo_bridge.commands
SET state='finished', response_code=$2, response_body=$3,
    response_type=$4, finished_at=now()
WHERE id=$1::uuid AND state='processing'
`, id, status, body, contentType)
    if err != nil { log.Printf("[vercelbridge] cannot persist result %s: %v", id, err) }
}

func pause(ctx context.Context, d time.Duration) {
    t := time.NewTimer(d)
    defer t.Stop()
    select {
    case <-ctx.Done():
    case <-t.C:
    }
}

// AcquireSingleton holds a session-level PostgreSQL advisory lock throughout
// the daemon process. Do not use transaction-pooling DSNs for this connection.
func AcquireSingleton(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
    conn, err := db.Conn(ctx)
    if err != nil { return nil, err }
    var acquired bool
    if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1, $2)", 711719, 1).Scan(&acquired); err != nil {
        conn.Close()
        return nil, fmt.Errorf("acquire advisory lock: %w", err)
    }
    if !acquired {
        conn.Close()
        return nil, errors.New("another Evolution Go daemon holds the WhatsApp singleton lock")
    }
    return conn, nil
}

// MonitorSingleton verifies the pinned connection stays alive. If it is lost,
// the caller MUST terminate the process; otherwise duplicate WA clients can run.
func MonitorSingleton(ctx context.Context, conn *sql.Conn) error {
    t := time.NewTicker(5*time.Second)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-t.C:
            probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
            var n int
            err := conn.QueryRowContext(probeCtx, "SELECT 1").Scan(&n)
            cancel()
            if err != nil || n != 1 { return fmt.Errorf("singleton lock connection lost: %v", err) }
        }
    }
}
