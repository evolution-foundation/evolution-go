package webhook_producer

// Durable webhook delivery (opt-in: WEBHOOK_DURABLE_QUEUE=true).
//
// The default producer retries in memory (5 attempts, 30 s apart): if the
// receiver stays down for more than ~2.5 min, or if this process restarts in the
// middle, the event is lost. With the queue on, every event is stored in the
// users DB (table webhook_pending) before the first attempt and deleted when the
// receiver answers 2xx. A background deliverer resumes whatever is still pending,
// also after a restart, until WEBHOOK_RETENTION (default 72h).
//
// Off (the default) nothing changes: no table, no goroutine, same retry as before.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	producer_interfaces "github.com/evolution-foundation/evolution-go/pkg/events/interfaces"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
)

const (
	defaultWebhookRetention      = 72 * time.Hour
	defaultWebhookTimeoutSeconds = 10
	// defaultWebhookRetryDelays: waits after each failed attempt; after the last
	// one it repeats until the retention is over (~75 attempts in 72h).
	defaultWebhookRetryDelays = "5s,15s,1m,5m,15m,1h"
	// leaseDuration: an attempt that takes longer than this (process died in the
	// middle) is picked up again by the deliverer.
	leaseDuration = 2 * time.Minute
	// keepFinishedFor: refused and expired rows stay this long for diagnosis.
	keepFinishedFor = 7 * 24 * time.Hour
	// maxLoggedResponseBody caps what goes to the log: the response may echo
	// message content.
	maxLoggedResponseBody = 200
	maxResponseBodyBytes  = 64 * 1024

	statusPending = "pending"
	statusRefused = "refused"
	statusExpired = "expired"
)

// WebhookPending is one event waiting to be delivered.
type WebhookPending struct {
	ID          uint       `gorm:"primaryKey"`
	URL         string     `gorm:"type:text;not null"`
	UserID      string     `gorm:"type:varchar(100);index"`
	Body        []byte     `gorm:"not null"`
	Status      string     `gorm:"type:varchar(20);not null;default:pending;index"`
	Attempts    int        `gorm:"not null;default:0"`
	NextAt      time.Time  `gorm:"not null;index"`
	LeasedUntil *time.Time `gorm:""`
	LastError   string     `gorm:"type:varchar(300)"`
	CreatedAt   time.Time  `gorm:"not null;index"`
}

func (WebhookPending) TableName() string { return "webhook_pending" }

// durableQueue is what the deliverer needs from storage (a fake in unit tests).
type durableQueue interface {
	Store(url, userID string, body []byte, now time.Time) (uint, error)
	// Lease marks the row as being delivered now; false if it is not pending, not
	// due yet or already leased by someone else.
	Lease(id uint, now time.Time) (*WebhookPending, bool, error)
	Delivered(id uint) error
	Refused(id uint, reason string) error
	Retry(id uint, attempts int, next time.Time, reason string) error
	Due(now time.Time, limit int) ([]uint, error)
	// Expire marks pending rows older than retention as expired (returns how many)
	// and deletes finished rows older than keepFinishedFor.
	Expire(now time.Time, retention time.Duration) (int64, error)
}

// durableDelivery stores, delivers and resumes events. Nil on the producer: off.
type durableDelivery struct {
	queue         durableQueue
	client        *http.Client
	retryDelays   []time.Duration
	retention     time.Duration
	now           func() time.Time
	loggerWrapper *logger_wrapper.LoggerManager
}

// DurableQueueEnabled reads WEBHOOK_DURABLE_QUEUE (default false).
func DurableQueueEnabled() bool {
	on, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("WEBHOOK_DURABLE_QUEUE")))
	return err == nil && on
}

// EnableDurableQueue creates the table and starts the deliverer. On error the
// producer keeps the in-memory retry.
func EnableDurableQueue(producer producer_interfaces.Producer, db *gorm.DB) error {
	p, ok := producer.(*webhookProducer)
	if !ok {
		return errors.New("not a webhook producer")
	}
	queue, err := newGormQueue(db)
	if err != nil {
		return err
	}
	p.durable = newDurableDelivery(queue, p.loggerWrapper)
	go p.durable.deliverer(nil)
	return nil
}

func newDurableDelivery(queue durableQueue, loggerWrapper *logger_wrapper.LoggerManager) *durableDelivery {
	return &durableDelivery{
		queue:         queue,
		client:        &http.Client{Timeout: webhookTimeout()},
		retryDelays:   webhookRetryDelays(loggerWrapper),
		retention:     webhookRetention(),
		now:           time.Now,
		loggerWrapper: loggerWrapper,
	}
}

// enqueue stores the event and tries it right away. False if it could not be
// stored (database down): the caller falls back to the in-memory retry.
func (d *durableDelivery) enqueue(url string, body []byte, userID string) bool {
	id, err := d.queue.Store(url, userID, body, d.now())
	if err != nil {
		d.logError(userID, "[%s] webhook queue unavailable, retrying in memory: %v", userID, err)
		return false
	}
	go d.attempt(id)
	return true
}

// delayFor is the wait after the n-th failed attempt: the configured delays and
// then the last one again, until the retention is over.
func (d *durableDelivery) delayFor(attempts int) time.Duration {
	if len(d.retryDelays) == 0 {
		return time.Minute
	}
	i := attempts - 1
	if i < 0 {
		i = 0
	}
	if i >= len(d.retryDelays) {
		i = len(d.retryDelays) - 1
	}
	return d.retryDelays[i]
}

// attempt delivers one stored event once, if it is due and not leased.
func (d *durableDelivery) attempt(id uint) {
	now := d.now()
	row, ok, err := d.queue.Lease(id, now)
	if err != nil {
		d.logError("webhook", "[webhook] could not lease event %d: %v", id, err)
		return
	}
	if !ok {
		return
	}
	responseBody, statusCode, retryable, err := d.post(row.URL, row.Body)
	if err == nil {
		if derr := d.queue.Delivered(id); derr != nil {
			d.logError(row.UserID, "[%s] webhook delivered but not removed from the queue (%d): %v", row.UserID, id, derr)
		}
		d.logInfo(row.UserID, "[%s] webhook sent successfully - url: %s, status: %d, attempt: %d", row.UserID, withoutSecret(row.URL), statusCode, row.Attempts+1)
		return
	}
	if !retryable {
		_ = d.queue.Refused(id, err.Error())
		d.logError(row.UserID, "[%s] webhook refused (status %d), not retried - url: %s, response: %s", row.UserID, statusCode, withoutSecret(row.URL), truncateForLog(responseBody))
		return
	}
	attempts := row.Attempts + 1
	wait := d.delayFor(attempts)
	_ = d.queue.Retry(id, attempts, now.Add(wait), err.Error())
	d.logWarn(row.UserID, "[%s] webhook failed - url: %s, attempt: %d, status: %d, error: %v, next in %s",
		row.UserID, withoutSecret(row.URL), attempts, statusCode, err, wait)
}

// post sends the payload once. retryable is false only for a response a retry
// cannot fix: 4xx except 408 (timeout) and 429 (rate limited).
func (d *durableDelivery) post(url string, body []byte) ([]byte, int, bool, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, 0, true, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, 0, true, fmt.Errorf("could not read the response: %v", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return responseBody, resp.StatusCode, true, nil
	}
	retryable := resp.StatusCode < 400 || resp.StatusCode >= 500 ||
		resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests
	return responseBody, resp.StatusCode, retryable, errors.New("received non-2xx response: " + resp.Status)
}

// deliverer resumes due events every 2 s and expires old ones every 10 min.
func (d *durableDelivery) deliverer(stop <-chan struct{}) {
	tick := time.NewTicker(2 * time.Second)
	expire := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	defer expire.Stop()
	d.expireOld()
	for {
		select {
		case <-stop:
			return
		case <-expire.C:
			d.expireOld()
		case <-tick.C:
			ids, err := d.queue.Due(d.now(), 50)
			if err != nil {
				d.logError("webhook", "[webhook] could not read pending events: %v", err)
				continue
			}
			for _, id := range ids {
				d.attempt(id)
			}
		}
	}
}

func (d *durableDelivery) expireOld() {
	lost, err := d.queue.Expire(d.now(), d.retention)
	if err != nil {
		d.logError("webhook", "[webhook] could not expire old events: %v", err)
		return
	}
	if lost > 0 {
		d.logError("webhook", "[webhook] %d events expired undelivered after %s", lost, d.retention)
	}
}

// ── storage ──────────────────────────────────────────────────────

type gormQueue struct{ db *gorm.DB }

func newGormQueue(db *gorm.DB) (*gormQueue, error) {
	if db == nil {
		return nil, errors.New("no database")
	}
	if err := db.AutoMigrate(&WebhookPending{}); err != nil {
		return nil, err
	}
	return &gormQueue{db: db}, nil
}

func (q *gormQueue) Store(url, userID string, body []byte, now time.Time) (uint, error) {
	row := WebhookPending{URL: url, UserID: userID, Body: body, Status: statusPending, NextAt: now, CreatedAt: now}
	if err := q.db.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (q *gormQueue) Lease(id uint, now time.Time) (*WebhookPending, bool, error) {
	until := now.Add(leaseDuration)
	res := q.db.Model(&WebhookPending{}).
		Where("id = ? AND status = ? AND next_at <= ? AND (leased_until IS NULL OR leased_until < ?)", id, statusPending, now, now).
		Update("leased_until", until)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	var row WebhookPending
	if err := q.db.First(&row, id).Error; err != nil {
		return nil, false, err
	}
	return &row, true, nil
}

func (q *gormQueue) Delivered(id uint) error {
	return q.db.Delete(&WebhookPending{}, id).Error
}

func (q *gormQueue) Refused(id uint, reason string) error {
	return q.db.Model(&WebhookPending{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": statusRefused, "last_error": clip(reason), "leased_until": nil}).Error
}

func (q *gormQueue) Retry(id uint, attempts int, next time.Time, reason string) error {
	return q.db.Model(&WebhookPending{}).Where("id = ?", id).
		Updates(map[string]interface{}{"attempts": attempts, "next_at": next, "last_error": clip(reason), "leased_until": nil}).Error
}

func (q *gormQueue) Due(now time.Time, limit int) ([]uint, error) {
	var ids []uint
	err := q.db.Model(&WebhookPending{}).
		Where("status = ? AND next_at <= ? AND (leased_until IS NULL OR leased_until < ?)", statusPending, now, now).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).
		Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func (q *gormQueue) Expire(now time.Time, retention time.Duration) (int64, error) {
	res := q.db.Model(&WebhookPending{}).
		Where("status = ? AND created_at < ?", statusPending, now.Add(-retention)).
		Updates(map[string]interface{}{"status": statusExpired, "leased_until": nil})
	if res.Error != nil {
		return 0, res.Error
	}
	err := q.db.Where("status <> ? AND created_at < ?", statusPending, now.Add(-keepFinishedFor)).
		Delete(&WebhookPending{}).Error
	return res.RowsAffected, err
}

// ── settings and helpers ─────────────────────────────────────────

// webhookRetention reads WEBHOOK_RETENTION (a Go duration, default 72h).
func webhookRetention() time.Duration {
	raw := strings.TrimSpace(os.Getenv("WEBHOOK_RETENTION"))
	if raw == "" {
		return defaultWebhookRetention
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultWebhookRetention
	}
	return d
}

// webhookTimeout reads WEBHOOK_TIMEOUT_SECONDS (seconds, default 10).
func webhookTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("WEBHOOK_TIMEOUT_SECONDS"))
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultWebhookTimeoutSeconds * time.Second
	}
	return time.Duration(seconds) * time.Second
}

// webhookRetryDelays reads WEBHOOK_RETRY_DELAYS ("5s,15s,1m,..."). An invalid
// value falls back to the default and is reported in the log.
func webhookRetryDelays(loggerWrapper *logger_wrapper.LoggerManager) []time.Duration {
	raw := strings.TrimSpace(os.Getenv("WEBHOOK_RETRY_DELAYS"))
	if raw == "" {
		delays, _ := parseRetryDelays(defaultWebhookRetryDelays)
		return delays
	}
	delays, err := parseRetryDelays(raw)
	if err != nil {
		if loggerWrapper != nil {
			loggerWrapper.GetLogger("webhook").LogWarn("[webhook] invalid WEBHOOK_RETRY_DELAYS %q (%v), using default %q", raw, err, defaultWebhookRetryDelays)
		}
		delays, _ = parseRetryDelays(defaultWebhookRetryDelays)
	}
	return delays
}

func parseRetryDelays(raw string) ([]time.Duration, error) {
	parts := strings.Split(raw, ",")
	delays := make([]time.Duration, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty delay in %q", raw)
		}
		d, err := time.ParseDuration(part)
		if err != nil {
			return nil, err
		}
		if d < 0 {
			return nil, fmt.Errorf("negative delay %q", part)
		}
		delays = append(delays, d)
	}
	return delays, nil
}

func clip(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}

// withoutSecret drops the query string: receiver URLs often carry a token
// (?token=...), which must not go to the log.
func withoutSecret(url string) string {
	if i := strings.IndexAny(url, "?#"); i >= 0 {
		return url[:i] + "?…"
	}
	return url
}

// truncateForLog limits what is written to the log: the response body may carry
// message content, so only the first 200 characters are kept.
func truncateForLog(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "-"
	}
	runes := []rune(text)
	if len(runes) > maxLoggedResponseBody {
		return string(runes[:maxLoggedResponseBody]) + "...(truncated)"
	}
	return text
}

func (d *durableDelivery) logger(userID string) *logger_wrapper.Logger {
	if d.loggerWrapper == nil {
		return nil
	}
	return d.loggerWrapper.GetLogger(userID)
}

func (d *durableDelivery) logInfo(userID, format string, args ...interface{}) {
	if l := d.logger(userID); l != nil {
		l.LogInfo(format, args...)
	}
}

func (d *durableDelivery) logWarn(userID, format string, args ...interface{}) {
	if l := d.logger(userID); l != nil {
		l.LogWarn(format, args...)
	}
}

func (d *durableDelivery) logError(userID, format string, args ...interface{}) {
	if l := d.logger(userID); l != nil {
		l.LogError(format, args...)
	}
}
