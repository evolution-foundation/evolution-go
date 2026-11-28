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
// Delivery is at-least-once: every POST carries the X-Webhook-Id header (the
// same value on every attempt of the same event), so the receiver can drop a
// repeated delivery.
//
// Off (the default) nothing changes: no table, no goroutine, same retry as before.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
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
	// minimumLease: an attempt that takes longer than its lease (process died in
	// the middle) is picked up again. The lease is never shorter than the HTTP
	// timeout plus leaseMargin, so a slow request is not sent twice at once.
	minimumLease = 2 * time.Minute
	leaseMargin  = 30 * time.Second
	// keepFinishedFor: refused and expired rows stay this long after they
	// finished, for diagnosis.
	keepFinishedFor = 7 * 24 * time.Hour
	// deliveryBatch and deliveryWorkers: how many due events are read at once and
	// how many are sent at the same time, so a slow receiver does not hold the rest.
	deliveryBatch   = 50
	deliveryWorkers = 8
	// storeTimeout and queryTimeout: a stalled database returns an error (and the
	// event goes to the in-memory retry) instead of blocking the caller.
	storeTimeout = 5 * time.Second
	queryTimeout = 15 * time.Second
	// maxLoggedResponseBody caps what goes to the log: the response may echo
	// message content.
	maxLoggedResponseBody = 200
	maxResponseBodyBytes  = 64 * 1024

	webhookIDHeader = "X-Webhook-Id"

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
	// FinishedAt: when the row became refused or expired (cleanup counts from here).
	FinishedAt *time.Time `gorm:"index"`
}

func (WebhookPending) TableName() string { return "webhook_pending" }

// durableQueue is what the deliverer needs from storage (a fake in unit tests).
type durableQueue interface {
	Store(url, userID string, body []byte, now time.Time) (uint, error)
	// Lease marks the row as being delivered until `until`; false if it is not
	// pending, not due yet, older than notBefore (past the retention) or already
	// leased by someone else.
	Lease(id uint, now, until, notBefore time.Time) (*WebhookPending, bool, error)
	Delivered(id uint) error
	Refused(id uint, reason string, now time.Time) error
	Retry(id uint, attempts int, next time.Time, reason string) error
	// Due lists pending rows that can be sent now, the longest waiting first.
	Due(now, notBefore time.Time, limit int) ([]uint, error)
	// Expire marks pending rows older than retention as expired (returns how many)
	// and deletes finished rows that finished more than keepFinishedFor ago.
	Expire(now time.Time, retention time.Duration) (int64, error)
}

// durableDelivery stores, delivers and resumes events. Nil on the producer: off.
type durableDelivery struct {
	queue         durableQueue
	client        *http.Client
	retryDelays   []time.Duration
	retention     time.Duration
	lease         time.Duration
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
	timeout := webhookTimeout()
	lease := minimumLease
	if timeout+leaseMargin > lease {
		lease = timeout + leaseMargin
	}
	return &durableDelivery{
		queue:         queue,
		client:        &http.Client{Timeout: timeout},
		retryDelays:   webhookRetryDelays(loggerWrapper),
		retention:     webhookRetention(),
		lease:         lease,
		now:           time.Now,
		loggerWrapper: loggerWrapper,
	}
}

// enqueue stores the event and tries it right away. False if it could not be
// stored (database down or too slow): the caller falls back to the in-memory retry.
func (d *durableDelivery) enqueue(url string, body []byte, userID string) bool {
	id, err := d.queue.Store(url, userID, body, d.now())
	if err != nil {
		d.logError(userID, "[%s] webhook queue unavailable, retrying in memory: %s", userID, redactError(err, url))
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

// attempt delivers one stored event once, if it is due, not leased and not past
// the retention.
func (d *durableDelivery) attempt(id uint) {
	now := d.now()
	row, ok, err := d.queue.Lease(id, now, now.Add(d.lease), now.Add(-d.retention))
	if err != nil {
		d.logError("webhook", "[webhook] could not lease event %d: %v", id, err)
		return
	}
	if !ok {
		return
	}
	safeURL := withoutSecret(row.URL)
	responseBody, statusCode, retryable, err := d.post(row.ID, row.URL, row.Body)
	// The wait counts from the end of the attempt: a slow receiver does not eat it.
	finished := d.now()
	if err == nil {
		if derr := d.queue.Delivered(id); derr != nil {
			d.logError(row.UserID, "[%s] webhook %d delivered but not removed from the queue (it may be sent again): %v", row.UserID, id, derr)
			return
		}
		d.logInfo(row.UserID, "[%s] webhook sent successfully - url: %s, status: %d, attempt: %d", row.UserID, safeURL, statusCode, row.Attempts+1)
		return
	}
	reason := redactError(err, row.URL)
	if !retryable {
		if uerr := d.queue.Refused(id, reason, finished); uerr != nil {
			d.logError(row.UserID, "[%s] webhook %d refused (status %d) but the refusal was not recorded, it will be tried again: %v", row.UserID, id, statusCode, uerr)
			return
		}
		d.logError(row.UserID, "[%s] webhook refused (status %d), not retried - url: %s, response: %s", row.UserID, statusCode, safeURL, truncateForLog(responseBody))
		return
	}
	attempts := row.Attempts + 1
	wait := d.delayFor(attempts)
	if uerr := d.queue.Retry(id, attempts, finished.Add(wait), reason); uerr != nil {
		d.logError(row.UserID, "[%s] webhook %d failed and the next attempt was not recorded (it is tried again when the lease ends): %v", row.UserID, id, uerr)
		return
	}
	d.logWarn(row.UserID, "[%s] webhook failed - url: %s, attempt: %d, status: %d, error: %s, next in %s",
		row.UserID, safeURL, attempts, statusCode, reason, wait)
}

// post sends the payload once. retryable is false only for a response a retry
// cannot fix: 4xx except 408 (timeout) and 429 (rate limited).
func (d *durableDelivery) post(id uint, url string, body []byte) ([]byte, int, bool, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhookIDHeader, strconv.FormatUint(uint64(id), 10))
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
			d.deliverDue()
		}
	}
}

// deliverDue sends one batch of due events, deliveryWorkers at a time, and waits
// for the batch: a slow receiver holds one worker, not the whole queue.
func (d *durableDelivery) deliverDue() {
	now := d.now()
	ids, err := d.queue.Due(now, now.Add(-d.retention), deliveryBatch)
	if err != nil {
		d.logError("webhook", "[webhook] could not read pending events: %v", err)
		return
	}
	workers := make(chan struct{}, deliveryWorkers)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		workers <- struct{}{}
		go func(id uint) {
			defer wg.Done()
			defer func() { <-workers }()
			d.attempt(id)
		}(id)
	}
	wg.Wait()
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

func (q *gormQueue) with(timeout time.Duration) (*gorm.DB, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return q.db.WithContext(ctx), cancel
}

func (q *gormQueue) Store(url, userID string, body []byte, now time.Time) (uint, error) {
	db, cancel := q.with(storeTimeout)
	defer cancel()
	row := WebhookPending{URL: url, UserID: userID, Body: body, Status: statusPending, NextAt: now, CreatedAt: now}
	if err := db.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (q *gormQueue) Lease(id uint, now, until, notBefore time.Time) (*WebhookPending, bool, error) {
	db, cancel := q.with(queryTimeout)
	defer cancel()
	res := db.Model(&WebhookPending{}).
		Where("id = ? AND status = ? AND next_at <= ? AND created_at >= ? AND (leased_until IS NULL OR leased_until < ?)",
			id, statusPending, now, notBefore, now).
		Update("leased_until", until)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	var row WebhookPending
	if err := db.First(&row, id).Error; err != nil {
		return nil, false, err
	}
	return &row, true, nil
}

func (q *gormQueue) Delivered(id uint) error {
	db, cancel := q.with(queryTimeout)
	defer cancel()
	return db.Delete(&WebhookPending{}, id).Error
}

func (q *gormQueue) Refused(id uint, reason string, now time.Time) error {
	db, cancel := q.with(queryTimeout)
	defer cancel()
	return db.Model(&WebhookPending{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": statusRefused, "last_error": clip(reason), "leased_until": nil, "finished_at": now}).Error
}

func (q *gormQueue) Retry(id uint, attempts int, next time.Time, reason string) error {
	db, cancel := q.with(queryTimeout)
	defer cancel()
	return db.Model(&WebhookPending{}).Where("id = ?", id).
		Updates(map[string]interface{}{"attempts": attempts, "next_at": next, "last_error": clip(reason), "leased_until": nil}).Error
}

func (q *gormQueue) Due(now, notBefore time.Time, limit int) ([]uint, error) {
	db, cancel := q.with(queryTimeout)
	defer cancel()
	var ids []uint
	err := db.Model(&WebhookPending{}).
		Where("status = ? AND next_at <= ? AND created_at >= ? AND (leased_until IS NULL OR leased_until < ?)", statusPending, now, notBefore, now).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "next_at"}}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).
		Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func (q *gormQueue) Expire(now time.Time, retention time.Duration) (int64, error) {
	db, cancel := q.with(queryTimeout)
	defer cancel()
	res := db.Model(&WebhookPending{}).
		Where("status = ? AND created_at < ?", statusPending, now.Add(-retention)).
		Updates(map[string]interface{}{"status": statusExpired, "leased_until": nil, "finished_at": now})
	if res.Error != nil {
		return 0, res.Error
	}
	err := db.Where("status <> ? AND COALESCE(finished_at, created_at) < ?", statusPending, now.Add(-keepFinishedFor)).
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
// value (including a zero or negative delay) falls back to the default and is
// reported in the log.
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
		// A zero wait would hammer a receiver that is down.
		if d <= 0 {
			return nil, fmt.Errorf("delay %q must be positive", part)
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

// withoutSecret drops what may carry a credential: the user info
// (user:password@) and the query string (?token=...).
func withoutSecret(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		out := u.Scheme + "://" + u.Host + u.EscapedPath()
		if u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
			out += "?…"
		}
		return out
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i] + "?…"
	}
	return raw
}

// redactError writes the error without the receiver URL's credentials: the Go
// HTTP client repeats the full URL (query included) in its error message.
func redactError(err error, rawURL string) string {
	if err == nil {
		return ""
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Sprintf("%s %q: %v", urlErr.Op, withoutSecret(rawURL), urlErr.Err)
	}
	msg := err.Error()
	if rawURL != "" {
		msg = strings.ReplaceAll(msg, rawURL, withoutSecret(rawURL))
	}
	return msg
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
