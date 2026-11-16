// Package handler serves the chat-style sender UI: a small standalone page that
// talks to the regular REST API from the browser so messages can be sent without
// retyping a recipient every time.
//
// It is registered from main.go (not pkg/routes) because it needs *config.Config
// to bootstrap the page, mirroring how the passkey ceremony routes are wired.
//
// Routes:
//
//	GET /            -> 302 to /sender
//	GET /sender      -> the page
//	GET /sender/     -> the page
//	GET /chat        -> the page
package handler

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

const pagePath = "web/sender/index.html"

// bootstrapToken is replaced with a JSON blob before the page is sent.
const bootstrapToken = "__EVO_BOOTSTRAP__"

// bootstrap is handed to the page so it can configure itself without the user
// pasting credentials by hand.
type bootstrap struct {
	// APIKey is the global API key, populated ONLY for loopback requests.
	APIKey string `json:"apiKey"`
	// KeyInjected reports whether APIKey was filled in, so the page can explain
	// itself instead of silently showing an empty settings form.
	KeyInjected bool `json:"keyInjected"`
}

type senderHandler struct {
	config *config.Config

	// authDB is opened lazily and only to read whatsmeow_lid_map. main.go's
	// authDB handle is nil whenever POSTGRES_AUTH_DB is set, so we cannot borrow
	// it and open our own instead.
	authOnce sync.Once
	authDB   *sql.DB
}

// lidStore returns a handle to the whatsmeow auth database, or nil when the
// service is running on SQLite (no POSTGRES_AUTH_DB configured).
func (h *senderHandler) lidStore() *sql.DB {
	h.authOnce.Do(func() {
		if h.config.PostgresAuthDB == "" {
			return
		}
		db, err := sql.Open("postgres", h.config.PostgresAuthDB)
		if err != nil {
			return
		}
		db.SetMaxOpenConns(2)
		if err := db.Ping(); err != nil {
			_ = db.Close()
			return
		}
		h.authDB = db
	})
	return h.authDB
}

// resolveLIDs maps WhatsApp LIDs (privacy identifiers such as 186896156205308)
// back to real phone numbers using the mapping whatsmeow maintains while it
// syncs. Without this the chat UI can only label a conversation with the LID,
// which is meaningless to a human.
//
// Requires the global API key: the response discloses phone numbers, so it must
// not be readable by anything that can merely reach the port.
//
// @Summary Resolve WhatsApp LIDs to phone numbers
// @Description Translates WhatsApp LID privacy identifiers into phone numbers using the whatsmeow LID mapping. Requires the global API key because the response discloses phone numbers. Returns an empty object when the mapping is unavailable (SQLite deployments) or when no LID matches.
// @Tags Sender
// @Produce json
// @Param lids query string true "Comma-separated list of numeric LIDs"
// @Success 200 {object} map[string]string "Map of LID to phone number"
// @Failure 401 {object} gin.H "not authorized"
// @Router /sender/resolve-lids [get]
func (h *senderHandler) resolveLIDs(c *gin.Context) {
	if c.GetHeader("apikey") != h.config.GlobalApiKey || h.config.GlobalApiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	out := gin.H{}

	raw := strings.Split(c.Query("lids"), ",")
	var wanted []interface{}
	for _, v := range raw {
		v = strings.TrimSpace(v)
		// Digits only: these values are interpolated into a query, and a LID is
		// always numeric, so anything else is rejected outright.
		if v == "" || strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
			continue
		}
		wanted = append(wanted, v)
	}
	if len(wanted) == 0 {
		c.JSON(http.StatusOK, out)
		return
	}

	db := h.lidStore()
	if db == nil {
		c.JSON(http.StatusOK, out) // mapping unavailable; caller keeps showing the LID
		return
	}

	placeholders := make([]string, len(wanted))
	for i := range wanted {
		placeholders[i] = "$" + itoa(i+1)
	}
	query := "SELECT lid, pn FROM whatsmeow_lid_map WHERE lid IN (" + strings.Join(placeholders, ",") + ")"

	rows, err := db.QueryContext(c.Request.Context(), query, wanted...)
	if err != nil {
		c.JSON(http.StatusOK, out)
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var lid, pn string
		if err := rows.Scan(&lid, &pn); err == nil {
			out[lid] = pn
		}
	}
	c.JSON(http.StatusOK, out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// isLoopback reports whether the request came from this machine.
//
// The global API key is only ever embedded for loopback callers. RemoteIP is the
// real socket peer (unlike ClientIP, which trusts X-Forwarded-For and could be
// spoofed), so a LAN or proxied request never receives the key and has to use
// the settings form like any other client.
func isLoopback(c *gin.Context) bool {
	ip := net.ParseIP(c.RemoteIP())
	return ip != nil && ip.IsLoopback()
}

// keyAutofillDisabled lets an operator switch off key embedding entirely, even
// for loopback requests, by setting SENDER_DISABLE_KEY_AUTOFILL to a truthy
// value. Useful when the host is shared or reachable through a local tunnel.
// Read straight from the environment, mirroring PASSKEY_PUBLIC_URL.
func keyAutofillDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SENDER_DISABLE_KEY_AUTOFILL"))) {
	case "1", "true", "yes", "enabled":
		return true
	}
	return false
}

func (h *senderHandler) page(c *gin.Context) {
	raw, err := os.ReadFile(pagePath)
	if err != nil {
		c.String(http.StatusInternalServerError,
			"sender UI not found at %s — run the server from the repository root", pagePath)
		return
	}

	boot := bootstrap{}
	if isLoopback(c) && !keyAutofillDisabled() {
		boot.APIKey = h.config.GlobalApiKey
		boot.KeyInjected = boot.APIKey != ""
	}

	// json.Marshal escapes <, > and & to \u003c/\u003e/\u0026, so the blob is
	// safe to drop inside a <script type="application/json"> element.
	encoded, err := json.Marshal(boot)
	if err != nil {
		encoded = []byte("{}")
	}

	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8",
		[]byte(strings.Replace(string(raw), bootstrapToken, string(encoded), 1)))
}

// RegisterRoutes wires the sender UI onto the engine. Call from main.go after the
// main router is assigned.
func RegisterRoutes(eng *gin.Engine, cfg *config.Config) {
	h := &senderHandler{config: cfg}

	eng.GET("/sender", h.page)
	eng.GET("/sender/", h.page)
	eng.GET("/chat", h.page)
	eng.GET("/sender/resolve-lids", h.resolveLIDs)

	// Bare host lands on the chat UI. Without this "/" is a plain 404, and the
	// sender page is easy to miss because /manager (the prebuilt instance-admin
	// bundle) has no messaging screen and cannot link out to one.
	eng.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/sender")
	})
}
