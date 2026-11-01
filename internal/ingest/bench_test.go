package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/queue/queuetest"
)

const (
	benchParallelism = 4
	benchPerKey      = 1 << 30
	benchWindow      = 10 * time.Second
	benchFlush       = 5 * time.Second
	benchKey         = "fbx_benchkey"
)

// benchBody is ~300 bytes, with a unique idempotencyKey per request.
func benchBody(n uint64) []byte {
	return fmt.Appendf(nil, `{"message":"The export button on the reports screen does nothing when I tap it.\nI tried twice after updating and then restarted the app, but it still just sits there with no error.","userId":"user-48213","appVersion":"2.14.1","platform":"iOS","metadata":{"locale":"en-GB","screen":"reports","build":"4411"},"idempotencyKey":"bench-%d"}`, n)
}

func benchHandler(b *testing.B) http.Handler {
	b.Helper()
	conn, objs := queuetest.Embedded(b, queue.Limits{MaxMsgs: 50_000_000, MaxBytes: 8 << 30})
	e := Entry{KeyID: "key_1", TenantID: "tn_1", ChannelID: "ch_1"}
	raw, err := json.Marshal(e)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := objs.Keys.Put(context.Background(), HashKey(benchKey), raw); err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	b.Cleanup(func() { cancel(); wg.Wait() })

	cache := NewKeyCache(discardLog())
	lim := NewLimiter(benchPerKey, benchWindow)
	cn := NewCounters(conn.JS, objs.Counters, benchFlush, discardLog())
	wg.Add(3)
	go func() { defer wg.Done(); _ = cache.Run(ctx, objs.Keys) }()
	go func() { defer wg.Done(); lim.Run(ctx) }()
	go func() { defer wg.Done(); cn.Run(ctx) }()
	select {
	case <-cache.Ready():
	case <-time.After(waitTimeout):
		b.Fatal("key cache not ready")
	}
	return NewHandler(Deps{
		Cache: cache, Limiter: lim, Counters: cn, JS: conn.JS,
		Config: Config{MaxBodyBytes: 16384, MaxMessageChars: 5000, MaxMetadataBytes: 4096, PublishTimeout: 2 * time.Second},
		Log:    discardLog(),
	})
}

// reportLatency merges per-goroutine samples and reports throughput and p50/p99.
func reportLatency(b *testing.B, lats *[][]time.Duration, mu *sync.Mutex) {
	b.Helper()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "req/s")
	mu.Lock()
	defer mu.Unlock()
	var all []time.Duration
	for _, l := range *lats {
		all = append(all, l...)
	}
	if len(all) == 0 {
		return
	}
	slices.Sort(all)
	b.ReportMetric(float64(all[len(all)/2].Microseconds()), "p50-µs")
	b.ReportMetric(float64(all[len(all)*99/100].Microseconds()), "p99-µs")
}

func BenchmarkIngest(b *testing.B) {
	h := benchHandler(b)
	srv := httptest.NewServer(h)
	b.Cleanup(srv.Close)
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 2 * runtime.GOMAXPROCS(0) * benchParallelism}}
	b.Cleanup(client.CloseIdleConnections)

	b.Run("http", func(b *testing.B) {
		var seq atomic.Uint64
		var mu sync.Mutex
		var lats [][]time.Duration

		b.SetParallelism(benchParallelism)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			local := make([]time.Duration, 0, 1024)
			for pb.Next() {
				req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(benchBody(seq.Add(1))))
				if err != nil {
					b.Error(err)
					return
				}
				req.Header.Set("Authorization", "Bearer "+benchKey)
				req.Header.Set("Content-Type", "application/json")
				start := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					b.Error(err)
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				local = append(local, time.Since(start))
				if resp.StatusCode != http.StatusAccepted {
					b.Errorf("status %d, want 202", resp.StatusCode)
					return
				}
			}
			mu.Lock()
			lats = append(lats, local)
			mu.Unlock()
		})
		b.StopTimer()
		reportLatency(b, &lats, &mu)
	})

	b.Run("handler-only", func(b *testing.B) {
		var seq atomic.Uint64
		var mu sync.Mutex
		var lats [][]time.Duration
		b.SetParallelism(benchParallelism)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			local := make([]time.Duration, 0, 1024)
			for pb.Next() {
				r := httptest.NewRequest(http.MethodPost, "/v1/feedback", bytes.NewReader(benchBody(seq.Add(1)+(1<<40))))
				r.Header.Set("Authorization", "Bearer "+benchKey)
				w := httptest.NewRecorder()
				start := time.Now()
				h.ServeHTTP(w, r)
				local = append(local, time.Since(start))
				if w.Code != http.StatusAccepted {
					b.Errorf("status %d, want 202: %s", w.Code, w.Body)
					return
				}
			}
			mu.Lock()
			lats = append(lats, local)
			mu.Unlock()
		})
		b.StopTimer()
		reportLatency(b, &lats, &mu)
	})
}
