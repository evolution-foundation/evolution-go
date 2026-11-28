package webhook_producer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeQueue is an in-memory durableQueue with the same rules as the gorm one.
type fakeQueue struct {
	mu       sync.Mutex
	rows     map[uint]*WebhookPending
	next     uint
	storeErr error
	retryErr error
}

func newFakeQueue() *fakeQueue { return &fakeQueue{rows: map[uint]*WebhookPending{}} }

func (q *fakeQueue) Store(url, userID string, body []byte, now time.Time) (uint, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.storeErr != nil {
		return 0, q.storeErr
	}
	q.next++
	q.rows[q.next] = &WebhookPending{ID: q.next, URL: url, UserID: userID, Body: body, Status: statusPending, NextAt: now, CreatedAt: now}
	return q.next, nil
}

func (q *fakeQueue) Lease(id uint, now, until, notBefore time.Time) (*WebhookPending, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	r, ok := q.rows[id]
	if !ok || r.Status != statusPending || r.NextAt.After(now) || r.CreatedAt.Before(notBefore) ||
		(r.LeasedUntil != nil && r.LeasedUntil.After(now)) {
		return nil, false, nil
	}
	r.LeasedUntil = &until
	copia := *r
	return &copia, true, nil
}

func (q *fakeQueue) Delivered(id uint) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.rows, id)
	return nil
}

func (q *fakeQueue) Refused(id uint, reason string, now time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	r := q.rows[id]
	r.Status, r.LastError, r.LeasedUntil, r.FinishedAt = statusRefused, reason, nil, &now
	return nil
}

func (q *fakeQueue) Retry(id uint, attempts int, next time.Time, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.retryErr != nil {
		return q.retryErr
	}
	r := q.rows[id]
	r.Attempts, r.NextAt, r.LastError, r.LeasedUntil = attempts, next, reason, nil
	return nil
}

func (q *fakeQueue) Due(now, notBefore time.Time, limit int) ([]uint, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var ids []uint
	for id, r := range q.rows {
		if r.Status == statusPending && !r.NextAt.After(now) && !r.CreatedAt.Before(notBefore) &&
			(r.LeasedUntil == nil || !r.LeasedUntil.After(now)) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := q.rows[ids[i]], q.rows[ids[j]]
		if !a.NextAt.Equal(b.NextAt) {
			return a.NextAt.Before(b.NextAt)
		}
		return ids[i] < ids[j]
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (q *fakeQueue) Expire(now time.Time, retention time.Duration) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var n int64
	for id, r := range q.rows {
		if r.Status == statusPending && r.CreatedAt.Before(now.Add(-retention)) {
			r.Status, r.FinishedAt = statusExpired, &now
			n++
			continue
		}
		fim := r.CreatedAt
		if r.FinishedAt != nil {
			fim = *r.FinishedAt
		}
		if r.Status != statusPending && fim.Before(now.Add(-keepFinishedFor)) {
			delete(q.rows, id)
		}
	}
	return n, nil
}

func (q *fakeQueue) count(status string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, r := range q.rows {
		if r.Status == status {
			n++
		}
	}
	return n
}

func producerWithQueue(t *testing.T, q *fakeQueue) (*webhookProducer, *durableDelivery) {
	t.Helper()
	t.Setenv("WEBHOOK_RETRY_DELAYS", "1m,5m")
	p := NewWebhookProducer("", nil).(*webhookProducer)
	p.durable = newDurableDelivery(q, nil)
	return p, p.durable
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func TestDurableQueueIsOffByDefault(t *testing.T) {
	cases := map[string]bool{"": false, "false": false, "no": false, "1": true, "true": true, "TRUE": true}
	for raw, want := range cases {
		t.Setenv("WEBHOOK_DURABLE_QUEUE", raw)
		if got := DurableQueueEnabled(); got != want {
			t.Fatalf("WEBHOOK_DURABLE_QUEUE=%q: got %v, want %v", raw, got, want)
		}
	}
	if p := NewWebhookProducer("", nil).(*webhookProducer); p.durable != nil {
		t.Fatal("a new producer must keep the in-memory retry until EnableDurableQueue")
	}
}

func TestProduceStoresThenDeliversWithAnIdAndRemoves(t *testing.T) {
	var hits int32
	var ids sync.Map
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		ids.Store(r.Header.Get(webhookIDHeader), true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	q := newFakeQueue()
	p, _ := producerWithQueue(t, q)

	if err := p.Produce("inst.message", []byte(`{"event":"Message"}`), srv.URL+"?token=secret", "inst"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return atomic.LoadInt32(&hits) == 1 && q.count(statusPending) == 0 })
	if _, ok := ids.Load("1"); !ok {
		t.Fatal("every delivery carries X-Webhook-Id, so the receiver can drop a repeat")
	}
}

func TestReceiverDownThenBackDeliversFromTheQueue(t *testing.T) {
	var up atomic.Bool
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if !up.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	q := newFakeQueue()
	_, d := producerWithQueue(t, q)
	now := time.Now()
	d.now = func() time.Time { return now }

	id, _ := q.Store(srv.URL, "inst", []byte(`{}`), now)
	d.attempt(id)
	if q.count(statusPending) != 1 || q.rows[id].Attempts != 1 {
		t.Fatalf("a 503 must stay pending with 1 attempt, got %+v", q.rows[id])
	}
	if !q.rows[id].NextAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("next attempt should follow WEBHOOK_RETRY_DELAYS, got %v", q.rows[id].NextAt)
	}
	if ids, _ := q.Due(now, now.Add(-d.retention), 50); len(ids) != 0 {
		t.Fatalf("not due yet, got %v", ids)
	}
	// Receiver back (or process restarted): the deliverer picks it up and delivers.
	up.Store(true)
	now = now.Add(2 * time.Minute)
	d.deliverDue()
	if q.count(statusPending) != 0 || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expected delivery on the second attempt, hits=%d rows=%v", hits, q.rows)
	}
}

func TestNextAttemptCountsFromTheEndOfASlowRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	q := newFakeQueue()
	_, d := producerWithQueue(t, q)
	start := time.Now()
	calls := 0
	// The request "takes" 3 minutes: the clock is read before and after it.
	d.now = func() time.Time {
		calls++
		if calls == 1 {
			return start
		}
		return start.Add(3 * time.Minute)
	}
	id, _ := q.Store(srv.URL, "inst", []byte(`{}`), start)
	d.attempt(id)
	if want := start.Add(3 * time.Minute).Add(time.Minute); !q.rows[id].NextAt.Equal(want) {
		t.Fatalf("the wait must start when the request ends: got %v, want %v", q.rows[id].NextAt, want)
	}
}

func TestRetryKeepsTheLastDelayAndExpiresAfterRetention(t *testing.T) {
	q := newFakeQueue()
	_, d := producerWithQueue(t, q)
	if got := d.delayFor(1); got != time.Minute {
		t.Fatalf("first wait: %v", got)
	}
	if got := d.delayFor(9); got != 5*time.Minute {
		t.Fatalf("after the list ends the last wait repeats, got %v", got)
	}
	now := time.Now()
	old, _ := q.Store("http://x", "inst", []byte(`{}`), now.Add(-73*time.Hour))
	// Past the retention it is not delivered, even before the expiration sweep.
	if ids, _ := q.Due(now, now.Add(-d.retention), 50); len(ids) != 0 {
		t.Fatalf("an event past the retention must not be due, got %v", ids)
	}
	if _, ok, _ := q.Lease(old, now, now.Add(d.lease), now.Add(-d.retention)); ok {
		t.Fatal("an event past the retention must not be leased")
	}
	d.now = func() time.Time { return now }
	d.expireOld()
	if q.count(statusExpired) != 1 {
		t.Fatal("events older than WEBHOOK_RETENTION expire")
	}
}

func TestFinishedRowsAreKeptSevenDaysFromWhenTheyFinished(t *testing.T) {
	q := newFakeQueue()
	now := time.Now()
	// Created 10 days ago, refused 2 days ago: still kept for diagnosis.
	id, _ := q.Store("http://x", "inst", []byte(`{}`), now.Add(-10*24*time.Hour))
	_ = q.Refused(id, "401", now.Add(-2*24*time.Hour))
	if _, err := q.Expire(now, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if q.count(statusRefused) != 1 {
		t.Fatal("a refused row is kept 7 days after it was refused, not after it was created")
	}
	if _, err := q.Expire(now.Add(6*24*time.Hour), 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if q.count(statusRefused) != 0 {
		t.Fatal("after 7 days from the refusal the row is deleted")
	}
}

func TestRefused4xxIsNotRetried(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	q := newFakeQueue()
	_, d := producerWithQueue(t, q)
	id, _ := q.Store(srv.URL, "inst", []byte(`{}`), time.Now())
	d.attempt(id)
	if q.count(statusRefused) != 1 || q.rows[id].FinishedAt == nil {
		t.Fatalf("401 must be refused with the time it finished, got %+v", q.rows[id])
	}
}

func TestFailedStateUpdateLeavesTheLeaseToExpire(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	q := newFakeQueue()
	q.retryErr = errors.New("db down")
	_, d := producerWithQueue(t, q)
	now := time.Now()
	d.now = func() time.Time { return now }
	id, _ := q.Store(srv.URL, "inst", []byte(`{}`), now)
	d.attempt(id)
	r := q.rows[id]
	if r.Attempts != 0 || r.LeasedUntil == nil {
		t.Fatalf("a retry that was not recorded keeps the row as it was, leased, got %+v", r)
	}
}

func TestLeaseOutlastsTheHTTPTimeout(t *testing.T) {
	t.Setenv("WEBHOOK_TIMEOUT_SECONDS", "300")
	d := newDurableDelivery(newFakeQueue(), nil)
	if d.lease <= 300*time.Second {
		t.Fatalf("a slow request must not be leased to a second sender: lease %v", d.lease)
	}
	t.Setenv("WEBHOOK_TIMEOUT_SECONDS", "10")
	if d := newDurableDelivery(newFakeQueue(), nil); d.lease != minimumLease {
		t.Fatalf("short timeouts keep the minimum lease, got %v", d.lease)
	}
}

func TestLeasePreventsDoubleDelivery(t *testing.T) {
	q := newFakeQueue()
	now := time.Now()
	notBefore := now.Add(-time.Hour)
	id, _ := q.Store("http://x", "inst", []byte(`{}`), now)
	if _, ok, _ := q.Lease(id, now, now.Add(minimumLease), notBefore); !ok {
		t.Fatal("first lease must work")
	}
	if _, ok, _ := q.Lease(id, now.Add(time.Second), now.Add(minimumLease), notBefore); ok {
		t.Fatal("second lease while the first is running must fail")
	}
	later := now.Add(minimumLease + time.Second)
	if _, ok, _ := q.Lease(id, later, later.Add(minimumLease), notBefore); !ok {
		t.Fatal("an expired lease (process died mid-attempt) is taken again")
	}
}

func TestSlowReceiverDoesNotHoldTheWholeBatch(t *testing.T) {
	var running, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&running, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	q := newFakeQueue()
	_, d := producerWithQueue(t, q)
	now := time.Now()
	for i := 0; i < 16; i++ {
		_, _ = q.Store(srv.URL, "inst", []byte(`{}`), now)
	}
	began := time.Now()
	d.deliverDue()
	if q.count(statusPending) != 0 {
		t.Fatalf("all 16 delivered, %d left", q.count(statusPending))
	}
	if p := atomic.LoadInt32(&peak); p < 2 || p > deliveryWorkers {
		t.Fatalf("deliveries run in parallel up to %d at a time, peak was %d", deliveryWorkers, p)
	}
	if took := time.Since(began); took > 1200*time.Millisecond {
		t.Fatalf("16 slow requests one by one would take 1.6 s; took %v", took)
	}
}

func TestQueueDownFallsBackToMemoryRetry(t *testing.T) {
	q := newFakeQueue()
	q.storeErr = errors.New("db down")
	p, _ := producerWithQueue(t, q)
	var fallback []string
	p.inMemory = func(url string, _ []byte, _ string) { fallback = append(fallback, url) }
	if err := p.Produce("inst.message", []byte(`{}`), "http://x/hook", "inst"); err != nil {
		t.Fatal(err)
	}
	if len(fallback) != 1 || fallback[0] != "http://x/hook" {
		t.Fatalf("with the queue down the event goes to the in-memory retry, got %v", fallback)
	}
}

func TestZeroRetryDelayIsRejected(t *testing.T) {
	if _, err := parseRetryDelays("5s,0s"); err == nil {
		t.Fatal("a zero wait would hammer a receiver that is down")
	}
	t.Setenv("WEBHOOK_RETRY_DELAYS", "1s,0s")
	if got := webhookRetryDelays(nil); len(got) != 6 || got[0] != 5*time.Second {
		t.Fatalf("an invalid list falls back to the default, got %v", got)
	}
}

func TestWithoutSecretHidesTokenAndUserInfo(t *testing.T) {
	cases := map[string]string{
		"http://api:8000/hook/c?token=secret":             "http://api:8000/hook/c?…",
		"https://user:pass@hooks.example.com/in":          "https://hooks.example.com/in",
		"https://user:pass@hooks.example.com/in?k=v#frag": "https://hooks.example.com/in?…",
	}
	for raw, want := range cases {
		if got := withoutSecret(raw); got != want {
			t.Fatalf("withoutSecret(%q) = %q, want %q", raw, got, want)
		}
	}
	err := &url.Error{Op: "Post", URL: "https://hooks.example.com/in?token=secret", Err: errors.New("connection refused")}
	got := redactError(err, "https://user:pass@hooks.example.com/in?token=secret")
	if strings.Contains(got, "secret") || strings.Contains(got, "pass") || !strings.Contains(got, "connection refused") {
		t.Fatalf("the error keeps the cause without the credentials, got %q", got)
	}
}
