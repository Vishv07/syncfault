package sandbox

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LoopbackRemoteServer provides an in-sandbox TCP/HTTP file transport endpoint backed by sb.Remote.
// It inspects sandbox fault markers (fault-netcut.active and fault-netslow.active) on every request
// so network partitions and latency degradations behave deterministically even without root namespaces.
type LoopbackRemoteServer struct {
	sb       *Sandbox
	listener net.Listener
	server   *http.Server
	mu       sync.Mutex
	bytesXfer int64
}

// StartRemoteServer starts the embedded loopback server rooted at sb.Remote.
func StartRemoteServer(sb *Sandbox) (*LoopbackRemoteServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen loopback remote: %w", err)
	}
	rs := &LoopbackRemoteServer{
		sb:       sb,
		listener: ln,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/files/", rs.handleFiles)
	rs.server = &http.Server{Handler: mux}

	go func() {
		_ = rs.server.Serve(ln)
	}()
	return rs, nil
}

// URL returns the base HTTP endpoint of the loopback server.
func (r *LoopbackRemoteServer) URL() string {
	return "http://" + r.listener.Addr().String() + "/files/"
}

// BytesTransferred returns the cumulative payload bytes served or written through the remote endpoint.
func (r *LoopbackRemoteServer) BytesTransferred() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bytesXfer
}

// Close shuts down the loopback server.
func (r *LoopbackRemoteServer) Close() error {
	if r.server != nil {
		return r.server.Close()
	}
	return nil
}

func (r *LoopbackRemoteServer) handleFiles(w http.ResponseWriter, req *http.Request) {
	// Check if netcut fault is currently active
	if _, err := os.Stat(filepath.Join(r.sb.Work, "fault-netcut.active")); err == nil {
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				_ = conn.Close()
				return
			}
		}
		http.Error(w, "network cut active", http.StatusServiceUnavailable)
		return
	}

	// Check if netslow fault is currently active
	if raw, err := os.ReadFile(filepath.Join(r.sb.Work, "fault-netslow.active")); err == nil {
		delay := parseDelayMS(string(raw))
		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
	}

	rel := strings.TrimPrefix(req.URL.Path, "/files/")
	dst, err := r.sb.ConfineSide("remote", rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	switch req.Method {
	case http.MethodGet:
		data, err := os.ReadFile(dst)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		r.addBytes(int64(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)

	case http.MethodPut:
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		r.addBytes(int64(len(body)))
		w.WriteHeader(http.StatusCreated)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (r *LoopbackRemoteServer) addBytes(n int64) {
	r.mu.Lock()
	r.bytesXfer += n
	r.mu.Unlock()
}

func parseDelayMS(s string) int {
	for _, field := range strings.Fields(s) {
		if strings.HasPrefix(field, "delay_ms=") {
			n, _ := strconv.Atoi(strings.TrimPrefix(field, "delay_ms="))
			return n
		}
	}
	return 0
}
