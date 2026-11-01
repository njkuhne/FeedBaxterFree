package feedbaxter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 2 * time.Minute
	shutdownTimeout   = 15 * time.Second
	freshCloseEvery   = 10 * time.Millisecond // how often stop closes never-used connections
)

// group runs the components of one Run and records the first failure.
type group struct {
	wg     sync.WaitGroup
	failed chan error
}

func newGroup() *group { return &group{failed: make(chan error, 1)} }

func (g *group) fail(err error) {
	select {
	case g.failed <- err:
	default:
	}
}

// start runs f in a goroutine tracked by wait. A non-nil error is a failure of the whole Run.
func (g *group) start(name string, f func() error) {
	g.wg.Go(func() {
		if err := f(); err != nil {
			g.fail(fmt.Errorf("%s: %w", name, err))
		}
	})
}

func (g *group) wait() { g.wg.Wait() }

// httpServer is the listener and http.Server of one Run.
type httpServer struct {
	srv     *http.Server
	ln      net.Listener
	done    chan struct{} // closed when Serve has returned
	started bool

	mu    sync.Mutex
	fresh map[net.Conn]struct{} // connections that have not sent a request yet
}

// trackState records connections in StateNew. http.Server.Shutdown waits up to
// 5s for those (a client may dial a spare connection it never uses), so stop
// closes them: nothing has been read from them, so no request is lost.
func (s *httpServer) trackState(c net.Conn, st http.ConnState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st == http.StateNew {
		s.fresh[c] = struct{}{}
	} else {
		delete(s.fresh, c)
	}
}

// listen binds addr so that a bind error surfaces before anything else starts.
func listen(addr string, h http.Handler, log *slog.Logger) (*httpServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	s := &httpServer{
		ln:    ln,
		fresh: map[net.Conn]struct{}{},
		srv: &http.Server{
			Handler:           h,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
		},
		done: make(chan struct{}),
	}
	s.srv.ConnState = s.trackState
	return s, nil
}

func (s *httpServer) serve(g *group) {
	s.started = true
	go func() {
		defer close(s.done)
		if err := s.srv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			g.fail(fmt.Errorf("http: %w", err))
		}
	}()
}

// stop stops accepting, lets in-flight requests finish (so every 202 already
// published), and waits for Serve to return.
func (s *httpServer) stop(log *slog.Logger) {
	if !s.started {
		_ = s.ln.Close()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shut := make(chan error, 1)
	go func() { shut <- s.srv.Shutdown(ctx) }()
	tick := time.NewTicker(freshCloseEvery)
	defer tick.Stop()
	var err error
wait:
	for {
		s.closeFresh()
		select {
		case err = <-shut:
			break wait
		case <-tick.C:
		}
	}
	if err != nil {
		log.Warn("http shutdown timed out; closing connections", "err", err)
		_ = s.srv.Close()
	}
	<-s.done
}

// closeFresh closes connections that have not sent a request (a client that
// was mid-way through its first request gets a reset and no 202, so it retries).
func (s *httpServer) closeFresh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.fresh {
		_ = c.Close()
		delete(s.fresh, c)
	}
}

// newMux serves /healthz and /readyz; callers add the rest.
func newMux(ready <-chan struct{}) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-ready:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("ok\n"))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not_ready", "message": "The service is starting."})
		}
	})
	return mux
}
