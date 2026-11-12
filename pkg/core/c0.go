package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var _k1 = []byte{0x67, 0x34, 0x3e, 0x22, 0x96, 0xdc, 0x1b, 0x2e, 0x17, 0xc3, 0xe7, 0xef, 0x14, 0x73, 0x28, 0x6a, 0x32, 0x68, 0xda, 0xea, 0x78, 0xae, 0x3e, 0xc4, 0xa4, 0x2a, 0x92, 0x51, 0xf5, 0x7e, 0x72, 0xd9, 0x68, 0x89, 0x4e, 0x87, 0x06, 0x0d, 0x20, 0xc4, 0x9d, 0x26}
var _k0 = []byte{0x0f, 0x40, 0x4a, 0x52, 0xe5, 0xe6, 0x34, 0x01, 0x7b, 0xaa, 0x84, 0x8a, 0x7a, 0x00, 0x4d, 0x44, 0x57, 0x1e, 0xb5, 0x86, 0x0d, 0xda, 0x57, 0xab, 0xca, 0x4c, 0xfd, 0x24, 0x9b, 0x1a, 0x13, 0xad, 0x01, 0xe6, 0x20, 0xa9, 0x65, 0x62, 0x4d, 0xea, 0xff, 0x54}

var (
	_vmf9 string
	_z4    string
)

func _le() string {
	if _vmf9 != "" && _z4 != "" {
		return _ok(_vmf9, _z4)
	}
	parts := [...]string{"h", "tt", "ps", "://", "li", "ce", "nse", ".", "ev", "ol", "ut", "io", "nf", "ou", "nd", "at", "io", "n.", "co", "m.", "br"}
	var s string
	for _, p := range parts {
		s += p
	}
	return s
}

func _ok(enc, key string) string {
	encBytes := _3v(enc)
	keyBytes := _3v(key)
	if len(keyBytes) == 0 {
		return ""
	}
	out := make([]byte, len(encBytes))
	for i, b := range encBytes {
		out[i] = b ^ keyBytes[i%len(keyBytes)]
	}
	return string(out)
}

func _3v(s string) []byte {
	if len(s)%2 != 0 {
		return nil
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		b[i/2] = _o7(s[i])<<4 | _o7(s[i+1])
	}
	return b
}

func _o7(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

var _degi = &http.Client{Timeout: 10 * time.Second}

func _19(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func _dr6(path string, payload interface{}, _58hw string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := _le() + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", _58hw)
	req.Header.Set("X-Signature", _19(body, _58hw))

	return _degi.Do(req)
}

func _5ms(path string) (*http.Response, error) {
	url := _le() + path
	return _degi.Get(url)
}

func _31(path string, payload interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := _le() + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return _degi.Do(req)
}

func _a643(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var _6pu struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(b, &_6pu); err == nil {
		msg := _6pu.Message
		if msg == "" {
			msg = _6pu.Error
		}
		if msg != "" {
			return fmt.Errorf("%s (HTTP %d)", strings.ToLower(msg), resp.StatusCode)
		}
	}
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}

type RuntimeConfig struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Key        string    `gorm:"uniqueIndex;size:100;not null" json:"key"`
	Value      string    `gorm:"type:text;not null" json:"value"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (RuntimeConfig) TableName() string {
	return "runtime_configs"
}

const (
	ConfigKeyInstanceID = "instance_id"
	ConfigKeyAPIKey     = "api_key"
	ConfigKeyTier       = "tier"
	ConfigKeyCustomerID = "customer_id"
)

var _w6 *gorm.DB

func SetDB(db *gorm.DB) {
	_w6 = db
}

func MigrateDB() error {
	if _w6 == nil {
		return fmt.Errorf("core: database not set, call SetDB first")
	}
	return _w6.AutoMigrate(&RuntimeConfig{})
}

func _u8(key string) (string, error) {
	if _w6 == nil {
		return "", fmt.Errorf("core: database not set")
	}
	var _fxv9 RuntimeConfig
	_tn := _w6.Where("key = ?", key).First(&_fxv9)
	if _tn.Error != nil {
		return "", _tn.Error
	}
	return _fxv9.Value, nil
}

func _wkb6(key, value string) error {
	if _w6 == nil {
		return fmt.Errorf("core: database not set")
	}
	var _fxv9 RuntimeConfig
	_tn := _w6.Where("key = ?", key).First(&_fxv9)
	if _tn.Error != nil {
		return _w6.Create(&RuntimeConfig{Key: key, Value: value}).Error
	}
	return _w6.Model(&_fxv9).Update("value", value).Error
}

func _eda(key string) {
	if _w6 == nil {
		return
	}
	_w6.Where("key = ?", key).Delete(&RuntimeConfig{})
}

type RuntimeData struct {
	APIKey     string
	Tier       string
	CustomerID int
}

func _x5t5() (*RuntimeData, error) {
	_58hw, err := _u8(ConfigKeyAPIKey)
	if err != nil || _58hw == "" {
		return nil, fmt.Errorf("no license found")
	}

	_48, _ := _u8(ConfigKeyTier)
	customerIDStr, _ := _u8(ConfigKeyCustomerID)
	customerID, _ := strconv.Atoi(customerIDStr)

	return &RuntimeData{
		APIKey:     _58hw,
		Tier:       _48,
		CustomerID: customerID,
	}, nil
}

func _uk(rd *RuntimeData) error {
	if err := _wkb6(ConfigKeyAPIKey, rd.APIKey); err != nil {
		return err
	}
	if err := _wkb6(ConfigKeyTier, rd.Tier); err != nil {
		return err
	}
	if rd.CustomerID > 0 {
		if err := _wkb6(ConfigKeyCustomerID, strconv.Itoa(rd.CustomerID)); err != nil {
			return err
		}
	}
	return nil
}

func _oon() {
	_eda(ConfigKeyAPIKey)
	_eda(ConfigKeyTier)
	_eda(ConfigKeyCustomerID)
}

func _cxz8() (string, error) {
	id, err := _u8(ConfigKeyInstanceID)
	if err == nil && len(id) == 36 {
		return id, nil
	}

	id = _usf9()
	if id == "" {
		id, err = _i6()
		if err != nil {
			return "", err
		}
	}

	if err := _wkb6(ConfigKeyInstanceID, id); err != nil {
		return "", err
	}
	return id, nil
}

func _usf9() string {
	hostname, _ := os.Hostname()
	macAddr := _28()
	if hostname == "" && macAddr == "" {
		return ""
	}

	seed := hostname + "|" + macAddr
	h := make([]byte, 16)
	copy(h, []byte(seed))
	for i := 16; i < len(seed); i++ {
		h[i%16] ^= seed[i]
	}
	h[6] = (h[6] & 0x0f) | 0x40 // _gjx6 4
	h[8] = (h[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func _28() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		if len(iface.HardwareAddr) > 0 {
			return iface.HardwareAddr.String()
		}
	}
	return ""
}

func _i6() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

var _ct atomic.Value // set during activation

func init() {
	_ct.Store([]byte{0})
}

func ComputeSessionSeed(instanceName string, rc *RuntimeContext) []byte {
	if rc == nil || !rc._oc.Load() {
		return nil // Will cause panic in caller — intentional
	}
	h := sha256.New()
	h.Write([]byte(instanceName))
	h.Write([]byte(rc._58hw))
	salt, _ := _ct.Load().([]byte)
	h.Write(salt)
	return h.Sum(nil)[:16]
}

func ValidateRouteAccess(rc *RuntimeContext) uint64 {
	if rc == nil {
		return 0
	}
	h := rc.ContextHash()
	return binary.LittleEndian.Uint64(h[:8])
}

func DeriveInstanceToken(_gy string, rc *RuntimeContext) string {
	if rc == nil || !rc._oc.Load() {
		return ""
	}
	h := sha256.Sum256([]byte(_gy + rc._58hw))
	return _m1ns(h[:8])
}

func _m1ns(b []byte) string {
	const _is9 = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = _is9[v>>4]
		dst[i*2+1] = _is9[v&0x0f]
	}
	return string(dst)
}

func ActivateIntegrity(rc *RuntimeContext) {
	if rc == nil {
		return
	}
	h := sha256.Sum256([]byte(rc._58hw + rc._gy + "ev0"))
	_ct.Store(h[:])
}

const (
	hbInterval = 30 * time.Minute
)

type RuntimeContext struct {
	_58hw       string
	_acq string // GLOBAL_API_KEY from .env — used as token for licensing check
	_gy   string
	_oc       atomic.Bool
	_hz2y      [32]byte // Derived from activation — required by ValidateContext
	mu           sync.RWMutex
	_htd       string // Registration URL shown to users before activation
	_aq     string // Registration token for polling
	_48         string
	_gjx6      string
	_r4a      atomic.Int64 // Messages sent since last heartbeat
	_b5      atomic.Int64 // Messages received since last heartbeat
}

var _okcm atomic.Pointer[RuntimeContext]

func (rc *RuntimeContext) TrackMessage() {
	if rc != nil {
		rc._r4a.Add(1)
	}
}

func TrackMessageSent() {
	if rc := _okcm.Load(); rc != nil {
		rc._r4a.Add(1)
	}
}

func TrackMessageRecv() {
	if rc := _okcm.Load(); rc != nil {
		rc._b5.Add(1)
	}
}

func (rc *RuntimeContext) _7dj() int64 {
	return rc._r4a.Swap(0)
}

func (rc *RuntimeContext) ContextHash() [32]byte {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc._hz2y
}

func (rc *RuntimeContext) IsActive() bool {
	return rc._oc.Load()
}

func (rc *RuntimeContext) RegistrationURL() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc._htd
}

func (rc *RuntimeContext) APIKey() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc._58hw
}

func (rc *RuntimeContext) InstanceID() string {
	return rc._gy
}

func InitializeRuntime(_48, _gjx6, _acq string) *RuntimeContext {
	if _48 == "" {
		_48 = "evolution-go"
	}
	if _gjx6 == "" {
		_gjx6 = "unknown"
	}

	rc := &RuntimeContext{
		_48:         _48,
		_gjx6:      _gjx6,
		_acq: _acq,
	}

	id, err := _cxz8()
	if err != nil {
		log.Fatalf("[runtime] failed to initialize instance: %v", err)
	}
	rc._gy = id

	rd, err := _x5t5()
	if err == nil && rd.APIKey != "" {
		rc._58hw = rd.APIKey
		fmt.Printf("  ✓ License found: %s...%s\n", rd.APIKey[:8], rd.APIKey[len(rd.APIKey)-4:])

		rc._hz2y = sha256.Sum256([]byte(rc._58hw + rc._gy))
		rc._oc.Store(true)
		ActivateIntegrity(rc)
		fmt.Println("  ✓ License activated successfully")

		go func() {
			if err := _dyo(rc, _gjx6); err != nil {
				fmt.Printf("  ⚠ Remote activation notice failed (non-blocking): %v\n", err)
			}
		}()
	} else if rc._acq != "" {
		rc._58hw = rc._acq
		if err := _dyo(rc, _gjx6); err == nil {
			_uk(&RuntimeData{APIKey: rc._acq, Tier: _48})
			rc._hz2y = sha256.Sum256([]byte(rc._58hw + rc._gy))
			rc._oc.Store(true)
			ActivateIntegrity(rc)
			fmt.Printf("  ✓ GLOBAL_API_KEY accepted — license saved and activated\n")
		} else {
			rc._58hw = ""
			_ohr5()
			rc._oc.Store(false)
		}
	} else {
		if _1xj(rc, _gjx6) {
			fmt.Println("  ✓ License activated automatically via EVOLUTION_OPERATOR_EMAIL")
		} else {
			_ohr5()
			rc._oc.Store(false)
		}
	}

	_okcm.Store(rc)

	return rc
}

func _1xj(rc *RuntimeContext, _gjx6 string) bool {
	email := strings.TrimSpace(os.Getenv("EVOLUTION_OPERATOR_EMAIL"))
	if email == "" {
		return false
	}

	payload := map[string]string{
		"email":       email,
		"tier":        rc._48,
		"version":     _gjx6,
		"instance_id": rc._gy,
	}

	resp, err := _31("/v1/register/auto", payload)
	if err != nil {
		fmt.Printf("  ⚠ Auto-activation skipped — licensing server unreachable: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_6pu := _a643(resp)
		if resp.StatusCode == http.StatusNotFound {
			fmt.Printf("  ℹ Auto-activation skipped — email not registered yet (first time?). Falling back to manual flow.\n")
		} else {
			fmt.Printf("  ⚠ Auto-activation rejected (%d): %v. Falling back to manual flow.\n",
				resp.StatusCode, _6pu)
		}
		return false
	}

	var _tn struct {
		APIKey     string `json:"api_key"`
		CustomerID int    `json:"customer_id"`
		Tier       string `json:"tier"`
		Status     string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&_tn); err != nil {
		fmt.Printf("  ⚠ Auto-activation response malformed: %v\n", err)
		return false
	}
	if _tn.APIKey == "" {
		fmt.Printf("  ⚠ Auto-activation response missing api_key\n")
		return false
	}

	rc.mu.Lock()
	rc._58hw = _tn.APIKey
	rc.mu.Unlock()

	if err := _uk(&RuntimeData{
		APIKey:     _tn.APIKey,
		Tier:       rc._48,
		CustomerID: _tn.CustomerID,
	}); err != nil {
		fmt.Printf("  ⚠ Auto-activation: could not save license to disk: %v\n", err)
	}

	rc.mu.Lock()
	rc._hz2y = sha256.Sum256([]byte(rc._58hw + rc._gy))
	rc.mu.Unlock()
	rc._oc.Store(true)
	ActivateIntegrity(rc)
	return true
}

func _ohr5() {
	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════════════╗")
	fmt.Println("  ║              License Registration Required               ║")
	fmt.Println("  ╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("  Server starting without license.")
	fmt.Println("  API endpoints will return 503 until license is activated.")
	fmt.Println("  Use GET /license/register to get the registration URL.")
	fmt.Println()
}

func (rc *RuntimeContext) _1x(authCodeOrKey, _48 string, customerID int) error {
	_58hw, err := _vvw2(authCodeOrKey)
	if err != nil {
		return fmt.Errorf("key exchange failed: %w", err)
	}

	rc.mu.Lock()
	rc._58hw = _58hw
	rc._htd = ""
	rc._aq = ""
	rc.mu.Unlock()

	if err := _uk(&RuntimeData{
		APIKey:     _58hw,
		Tier:       _48,
		CustomerID: customerID,
	}); err != nil {
		fmt.Printf("  ⚠ Warning: could not save license: %v\n", err)
	}

	if err := _dyo(rc, rc._gjx6); err != nil {
		return err
	}

	rc.mu.Lock()
	rc._hz2y = sha256.Sum256([]byte(rc._58hw + rc._gy))
	rc.mu.Unlock()
	rc._oc.Store(true)
	ActivateIntegrity(rc)

	fmt.Printf("  ✓ License activated! Key: %s...%s (_48: %s)\n",
		_58hw[:8], _58hw[len(_58hw)-4:], _48)

	go func() {
		if err := _fkt(rc, 0); err != nil {
			fmt.Printf("  ⚠ First heartbeat failed: %v\n", err)
		}
	}()

	return nil
}

func ValidateContext(rc *RuntimeContext) (bool, string) {
	if rc == nil {
		return false, ""
	}
	if !rc._oc.Load() {
		return false, rc.RegistrationURL()
	}
	expected := sha256.Sum256([]byte(rc._58hw + rc._gy))
	actual := rc.ContextHash()
	if expected != actual {
		return false, ""
	}
	return true, ""
}

func GateMiddleware(rc *RuntimeContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if path == "/health" || path == "/server/ok" || path == "/favicon.ico" ||
			path == "/license/status" || path == "/license/register" || path == "/license/activate" ||
			strings.HasPrefix(path, "/manager") || strings.HasPrefix(path, "/assets") ||
			strings.HasPrefix(path, "/passkey-ceremony") ||
			strings.HasPrefix(path, "/swagger") || path == "/ws" ||
			strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".css") ||
			strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".png") ||
			strings.HasSuffix(path, ".ico") || strings.HasSuffix(path, ".woff2") ||
			strings.HasSuffix(path, ".woff") || strings.HasSuffix(path, ".ttf") {
			c.Next()
			return
		}

		valid, _ := ValidateContext(rc)
		if !valid {
			scheme := "http"
			if c.Request.TLS != nil {
				scheme = "https"
			}
			managerURL := fmt.Sprintf("%s://%s/manager/login", scheme, c.Request.Host)

			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":        "service not activated",
				"code":         "LICENSE_REQUIRED",
				"register_url": managerURL,
				"message":      "License required. Open the manager to activate your license.",
			})
			return
		}

		c.Set("_rch", rc.ContextHash())
		c.Next()
	}
}

func LicenseRoutes(eng *gin.Engine, rc *RuntimeContext) {
	lic := eng.Group("/license")
	{
		lic.GET("/status", func(c *gin.Context) {
			status := "inactive"
			if rc.IsActive() {
				status = "active"
			}

			resp := gin.H{
				"status":      status,
				"instance_id": rc._gy,
			}

			rc.mu.RLock()
			if rc._58hw != "" {
				resp["api_key"] = rc._58hw[:8] + "..." + rc._58hw[len(rc._58hw)-4:]
			}
			rc.mu.RUnlock()

			c.JSON(http.StatusOK, resp)
		})

		lic.GET("/register", func(c *gin.Context) {
			if rc.IsActive() {
				c.JSON(http.StatusOK, gin.H{
					"status":  "active",
					"message": "License is already active",
				})
				return
			}

			rc.mu.RLock()
			existingURL := rc._htd
			rc.mu.RUnlock()

			if existingURL != "" {
				c.JSON(http.StatusOK, gin.H{
					"status":       "pending",
					"register_url": existingURL,
				})
				return
			}

			payload := map[string]string{
				"tier":        rc._48,
				"version":     rc._gjx6,
				"instance_id": rc._gy,
			}
			if redirectURI := c.Query("redirect_uri"); redirectURI != "" {
				payload["redirect_uri"] = redirectURI
			}

			resp, err := _31("/v1/register/init", payload)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{
					"error":   "Failed to contact licensing server",
					"details": err.Error(),
				})
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				_6pu := _a643(resp)
				c.JSON(resp.StatusCode, gin.H{
					"error":   "Licensing server error",
					"details": _6pu.Error(),
				})
				return
			}

			var _h79q struct {
				RegisterURL string `json:"register_url"`
				Token       string `json:"token"`
			}
			json.NewDecoder(resp.Body).Decode(&_h79q)

			rc.mu.Lock()
			rc._htd = _h79q.RegisterURL
			rc._aq = _h79q.Token
			rc.mu.Unlock()

			fmt.Printf("  → Registration URL: %s\n", _h79q.RegisterURL)

			c.JSON(http.StatusOK, gin.H{
				"status":       "pending",
				"register_url": _h79q.RegisterURL,
			})
		})

		lic.GET("/activate", func(c *gin.Context) {
			if rc.IsActive() {
				c.JSON(http.StatusOK, gin.H{
					"status":  "active",
					"message": "License is already active",
				})
				return
			}

			code := c.Query("code")
			if code == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing code parameter",
					"message": "Provide ?code=AUTHORIZATION_CODE from the registration callback.",
				})
				return
			}

			exchangeResp, err := _31("/v1/register/exchange", map[string]string{
				"authorization_code": code,
				"instance_id":       rc._gy,
			})
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{
					"error":   "Failed to contact licensing server",
					"details": err.Error(),
				})
				return
			}
			defer exchangeResp.Body.Close()

			if exchangeResp.StatusCode != http.StatusOK {
				_6pu := _a643(exchangeResp)
				c.JSON(exchangeResp.StatusCode, gin.H{
					"error":   "Exchange failed",
					"details": _6pu.Error(),
				})
				return
			}

			var _tn struct {
				APIKey     string `json:"api_key"`
				Tier       string `json:"tier"`
				CustomerID int    `json:"customer_id"`
			}
			json.NewDecoder(exchangeResp.Body).Decode(&_tn)

			if _tn.APIKey == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid or expired code",
					"message": "The authorization code is invalid or has expired.",
				})
				return
			}

			if err := rc._1x(_tn.APIKey, _tn.Tier, _tn.CustomerID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Activation failed",
					"details": err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"status":  "active",
				"message": "License activated successfully!",
			})
		})
	}
}

func StartHeartbeat(ctx context.Context, rc *RuntimeContext, startTime time.Time) {
	go func() {
		ticker := time.NewTicker(hbInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !rc.IsActive() {
					continue
				}
				uptime := int64(time.Since(startTime).Seconds())
				if err := _fkt(rc, uptime); err != nil {
					fmt.Printf("  ⚠ Heartbeat failed (non-blocking): %v\n", err)
				}
			}
		}
	}()
}

func Shutdown(rc *RuntimeContext) {
	if rc == nil || rc._58hw == "" {
		return
	}
	_d01d(rc)
}

func _p19s(code string) (_58hw string, err error) {
	resp, err := _31("/v1/register/exchange", map[string]string{
		"authorization_code": code,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", _a643(resp)
	}

	var _tn struct {
		APIKey string `json:"api_key"`
	}
	json.NewDecoder(resp.Body).Decode(&_tn)
	if _tn.APIKey == "" {
		return "", fmt.Errorf("exchange returned empty api_key")
	}
	return _tn.APIKey, nil
}

func _vvw2(authCodeOrKey string) (string, error) {
	_58hw, err := _p19s(authCodeOrKey)
	if err == nil && _58hw != "" {
		return _58hw, nil
	}
	return authCodeOrKey, nil
}

func _dyo(rc *RuntimeContext, _gjx6 string) error {
	resp, err := _dr6("/v1/activate", map[string]string{
		"instance_id": rc._gy,
		"version":     _gjx6,
	}, rc._58hw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return _a643(resp)
	}

	var _tn struct {
		Status string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&_tn)

	if _tn.Status != "active" {
		return fmt.Errorf("activation returned status: %s", _tn.Status)
	}
	return nil
}

func _fkt(rc *RuntimeContext, uptimeSeconds int64) error {
	_r4a := rc._7dj()
	_b5 := rc._b5.Swap(0)

	payload := map[string]any{
		"instance_id":    rc._gy,
		"uptime_seconds": uptimeSeconds,
		"version":        rc._gjx6,
	}

	if _r4a > 0 || _b5 > 0 {
		bundle := map[string]any{}
		if _r4a > 0 {
			bundle["messages_sent"] = _r4a
		}
		if _b5 > 0 {
			bundle["messages_recv"] = _b5
		}
		payload["telemetry_bundle"] = bundle
	}

	resp, err := _dr6("/v1/heartbeat", payload, rc._58hw)
	if err != nil {
		rc._r4a.Add(_r4a)
		rc._b5.Add(_b5)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rc._r4a.Add(_r4a)
		rc._b5.Add(_b5)
		return _a643(resp)
	}
	return nil
}

func _d01d(rc *RuntimeContext) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, _ := json.Marshal(map[string]string{
		"instance_id": rc._gy,
	})

	url := _le() + "/v1/deactivate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", rc._58hw)
	req.Header.Set("X-Signature", _19(body, rc._58hw))
	_degi.Do(req)
}
