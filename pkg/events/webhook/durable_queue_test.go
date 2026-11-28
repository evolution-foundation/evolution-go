package webhook_producer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
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

func (q *fakeQueue) Lease(id uint, now time.Time) (*WebhookPending, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	r, ok := q.rows[id]
	if !ok || r.Status != statusPending || r.NextAt.After(now) || (r.LeasedUntil != nil && r.LeasedUntil.After(now)) {
		return nil, false, nil
	}
	until := now.Add(leaseDuration)
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

func (q *fakeQueue) Refused(id uint, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.rows[id].Status, q.rows[id].LastError, q.rows[id].LeasedUntil = statusRefused, reason, nil
	return nil
}

func (q *fakeQueue) Retry(id uint, attempts int, next time.Time, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	r := q.rows[id]
	r.Attempts, r.NextAt, r.LastError, r.LeasedUntil = attempts, next, reason, nil
	return nil
}

func (q *fakeQueue) Due(now time.Time, limit int) ([]uint, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var ids []uint
	for id, r := range q.rows {
		if r.Status == statusPending && !r.NextAt.After(now) && (r.LeasedUntil == nil || !r.LeasedUntil.After(now)) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (q *fakeQueue) Expire(now time.Time, retention time.Duration) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var n int64
	for _, r := range q.rows {
		if r.Status == statusPending && r.CreatedAt.Before(now.Add(-retention)) {
			r.Status = statusExpired
			n++
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
	for i := 0; i < 200; i++ {
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

func TestProduceStoresThenDeliversAndRemoves(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	q := newFakeQueue()
	p, _ := producerWithQueue(t, q)

	if err := p.Produce("inst.message", []byte(`{"event":"Message"}`), srv.URL+"?token=secret", "inst"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return atomic.LoadInt32(&hits) == 1 && q.count(statusPending) == 0 })
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
	if ids, _ := q.Due(now, 50); len(ids) != 0 {
		t.Fatalf("not due yet, got %v", ids)
	}
	// Receiver back (or process restarted): the deliverer picks it up and delivers.
	up.Store(true)
	now = now.Add(2 * time.Minute)
	ids, _ := q.Due(now, 50)
	for _, i := range ids {
		d.attempt(i)
	}
	if q.count(statusPending) != 0 || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expected delivery on the second attempt, hits=%d rows=%v", hits, q.rows)
	}
}

func TestRetryKeepsTheLastDelayUntilRetention(t *testing.T) {
	q := newFakeQueue()
	_, d := producerWithQueue(t, q)
	if got := d.delayFor(1); got != time.Minute {
		t.Fatalf("first wait: %v", got)
	}
	if got := d.delayFor(9); got != 5*time.Minute {
		t.Fatalf("after the list ends the last wait repeats, got %v", got)
	}
	now := time.Now()
	q.Store("http://x", "inst", []byte(`{}`), now.Add(-73*time.Hour))
	d.now = func() time.Time { return now }
	d.expireOld()
	if q.count(statusExpired) != 1 {
		t.Fatal("events older than WEBHOOK_RETENTION expire")
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
	if q.count(statusRefused) != 1 {
		t.Fatalf("401 must be refused, got %+v", q.rows[id])
	}
}

func TestLeasePreventsDoubleDelivery(t *testing.T) {
	q := newFakeQueue()
	now := time.Now()
	id, _ := q.Store("http://x", "inst", []byte(`{}`), now)
	if _, ok, _ := q.Lease(id, now); !ok {
		t.Fatal("first lease must work")
	}
	if _, ok, _ := q.Lease(id, now.Add(time.Second)); ok {
		t.Fatal("second lease while the first is running must fail")
	}
	if _, ok, _ := q.Lease(id, now.Add(leaseDuration+time.Second)); !ok {
		t.Fatal("an expired lease (process died mid-attempt) is taken again")
	}
}

func TestQueueDownFallsBackToMemoryRetry(t *testing.T) {
	q := newFakeQueue()
	q.storeErr = errors.New("db down")
	_, d := producerWithQueue(t, q)
	// false makes Produce use the in-memory retry instead of dropping the event.
	if d.enqueue("http://x", []byte(`{}`), "inst") {
		t.Fatal("enqueue must report that the event was not stored")
	}
}

func TestWithoutSecretHidesTheToken(t *testing.T) {
	got := withoutSecret("http://api:8000/hook/c?token=secret")
	if got != "http://api:8000/hook/c?…" {
		t.Fatalf("got %q", got)
	}
}
