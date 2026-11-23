// WorkiDigital: stateless HTTP gateway for the Evolution Go Vercel Daemon.
// This process never connects to WhatsApp. It queues authenticated JSON
// commands in a private schema of Supabase PostgreSQL.
package main

import (
    "crypto/sha256"
    "crypto/subtle"
    "database/sql"
    "encoding/hex"
    "encoding/json"
    "errors"
    "io"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/google/uuid"
    _ "github.com/lib/pq"
    "github.com/evolution-foundation/evolution-go/pkg/vercelbridge"
)

const maxRequestBytes = 256 << 10
const maxResponseBytes = 1 << 20
const waitDuration = 18 * time.Second

type gateway struct {
    db *sql.DB
    key string
    tokenKeyHex string
}

type job struct {
    State string
    Code sql.NullInt64
    Body []byte
    Type sql.NullString
    Started sql.NullTime
}

func main() {
    key, dsn := os.Getenv("BRIDGE_API_KEY"), os.Getenv("POSTGRES_USERS_DB")
    tokenKeyHex := os.Getenv("BRIDGE_TOKEN_KEY")
    if key == "" || dsn == "" { log.Fatal("BRIDGE_API_KEY and POSTGRES_USERS_DB are required") }
    if err := vercelbridge.ValidateTokenKey(tokenKeyHex); err != nil { log.Fatal(err) }
    db, err := sql.Open("postgres", dsn)
    if err != nil { log.Fatal(err) }
    defer db.Close()
    db.SetMaxOpenConns(8)
    db.SetMaxIdleConns(2)
    db.SetConnMaxLifetime(4*time.Minute)
    if err := db.Ping(); err != nil { log.Fatalf("database connection failed: %v", err) }
    port := os.Getenv("PORT")
    if port == "" { port = "8080" }
    srv := &http.Server{
        Addr: ":"+port,
        Handler: &gateway{db:db,key:key,tokenKeyHex:tokenKeyHex},
        ReadHeaderTimeout: 8*time.Second,
        ReadTimeout: 20*time.Second,
        WriteTimeout: 24*time.Second,
    }
    log.Printf("[vercel-api] listening on port %s", port)
    log.Fatal(srv.ListenAndServe())
}

func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Cache-Control", "no-store")
    if r.URL.Path == "/health" {
        w.Header().Set("Content-Type","application/json")
        io.WriteString(w, `{"status":"ok","component":"evolution-go-vercel-gateway"}`)
        return
    }
    if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Bridge-Key")), []byte(g.key)) != 1 {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    if strings.HasPrefix(r.URL.Path, "/bridge/jobs/") {
        if r.Method != http.MethodGet { http.Error(w,"method not allowed",405); return }
        id := strings.TrimPrefix(r.URL.Path, "/bridge/jobs/")
        if _,err := uuid.Parse(id); err != nil { http.Error(w,"invalid job id",400); return }
        g.writeJob(w,r,id)
        return
    }
    if !allowPath(r.URL.Path) || !allowMethod(r.Method) {
        http.Error(w,"not supported by JSON command bridge",404)
        return
    }
    if r.Header.Get("Content-Encoding") != "" && r.Header.Get("Content-Encoding") != "identity" {
        http.Error(w,"compressed request bodies are not supported",415)
        return
    }
    if r.Method != http.MethodGet && r.Method != http.MethodDelete &&
        !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")),"application/json") {
        http.Error(w,"use application/json",415)
        return
    }
    data,err := io.ReadAll(io.LimitReader(r.Body,maxRequestBytes+1))
    if err != nil { http.Error(w,"invalid request body",400); return }
    if len(data)>maxRequestBytes { http.Error(w,"request exceeds 256 KiB",413); return }

    authKey:=r.Header.Get("apikey")
    if authKey=="" || len(authKey)>4096 { http.Error(w,"missing or invalid apikey",401); return }
    encrypted,err:=vercelbridge.SealAPIKey(authKey,g.tokenKeyHex)
    if err!=nil { http.Error(w,"bridge key configuration invalid",503); return }
    uri := r.URL.RequestURI()
    sum:=sha256.Sum256(append([]byte(r.Method+"\n"+uri+"\n"+authKey+"\n"),data...))
    digest:=hex.EncodeToString(sum[:])
    id:=uuid.New()
    if key := strings.TrimSpace(r.Header.Get("X-Idempotency-Key")); key!="" {
        if len(key)>128 { http.Error(w,"idempotency key too long",400); return }
        id=uuid.NewSHA1(uuid.NameSpaceOID,[]byte(r.Method+"\n"+uri+"\n"+key))
    }
    _,err=g.db.ExecContext(r.Context(),`
INSERT INTO evo_bridge.commands (id,method,request_path,request_body,request_sha256,auth_ciphertext)
VALUES ($1::uuid,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING
`,id.String(),r.Method,uri,data,digest,encrypted)
    if err!=nil {
        log.Printf("[vercel-api] queue write failed: %v",err)
        http.Error(w,"command queue unavailable",503)
        return
    }
    // Existing idempotency keys must never silently run different bodies.
    var storedDigest string
    if err:=g.db.QueryRowContext(r.Context(),
        "SELECT request_sha256 FROM evo_bridge.commands WHERE id=$1::uuid",id.String()).Scan(&storedDigest);err!=nil {
        http.Error(w,"job lookup unavailable",503);return
    }
    if storedDigest!=digest { http.Error(w,"idempotency key reused with different payload",409);return }
    deadline:=time.NewTimer(waitDuration)
    defer deadline.Stop()
    tick:=time.NewTicker(350*time.Millisecond)
    defer tick.Stop()
    for {
        j,err:=lookup(r,id.String(),g.db)
        if err==nil && j.State=="finished" {
            sendFinished(w,j);return
        }
        if err!=nil {
            log.Printf("[vercel-api] job lookup failed: %v",err)
            http.Error(w,"job lookup unavailable",503);return
        }
        select {
        case <-deadline.C:
            pending(w,id.String(),j.State);return
        case <-r.Context().Done():
            return
        case <-tick.C:
        }
    }
}

func (g *gateway) writeJob(w http.ResponseWriter,r *http.Request,id string) {
    j,err:=lookup(r,id,g.db)
    if errors.Is(err,sql.ErrNoRows) { http.Error(w,"job not found",404);return }
    if err!=nil { http.Error(w,"job lookup unavailable",503);return }
    if j.State=="finished" { sendFinished(w,j);return }
    pending(w,id,j.State)
}

func lookup(r *http.Request,id string,db *sql.DB)(job,error){
    j:=job{}
    err:=db.QueryRowContext(r.Context(),`
SELECT state,response_code,response_body,response_type,started_at
FROM evo_bridge.commands WHERE id=$1::uuid
`,id).Scan(&j.State,&j.Code,&j.Body,&j.Type,&j.Started)
    return j,err
}
func sendFinished(w http.ResponseWriter,j job){
    if len(j.Body)>maxResponseBytes { http.Error(w,"response exceeds bridge limit",502);return }
    if j.Type.Valid && (strings.HasPrefix(j.Type.String,"application/json") || strings.HasPrefix(j.Type.String,"text/plain")) {
        w.Header().Set("Content-Type",j.Type.String)
    } else { w.Header().Set("Content-Type","application/octet-stream") }
    code:=http.StatusBadGateway
    if j.Code.Valid && j.Code.Int64>=100 && j.Code.Int64<=599 {code=int(j.Code.Int64)}
    w.WriteHeader(code)
    w.Write(j.Body)
}
func pending(w http.ResponseWriter,id,state string) {
    w.Header().Set("Content-Type","application/json")
    w.Header().Set("Retry-After","2")
    w.Header().Set("Location","/bridge/jobs/"+id)
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]string{
        "job_id":id, "status":state,
        "hint":"check GET /bridge/jobs/{job_id}; do not blindly retry writes",
    })
}
func allowMethod(m string)bool{
    switch m {
    case "GET","POST","PUT","PATCH","DELETE":return true
    }
    return false
}
func allowPath(p string)bool{
    // Explicitly omit /manager, /ws, /assets, /passkey and all license routes.
    if p=="/server/ok" { return true }
    // Upstream license activation routes stay behind X-Bridge-Key.
    // Use the original GLOBAL_API_KEY in apikey on these requests.
    if p=="/license/status" || p=="/license/register" || p=="/license/activate" { return true }
    for _,prefix:=range []string{
        "/instance/","/send/","/message/","/group/","/chat/","/user/",
        "/community/","/label/","/unlabel/","/newsletter/","/call/","/polls/",
    } {
        if strings.HasPrefix(p,prefix) { return true }
    }
    return false
}
