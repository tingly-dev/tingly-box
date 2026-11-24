package capture

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Handler wraps next. It must be the outermost handler so every request is
// seen before routing or body binding, including ones next rejects. It never
// reads the request body and never changes what next sees or answers (apart
// from the opt-in request-id header).
func (t *Tap) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e := &exchange{seq: t.reqSeq.Add(1), t: time.Now().UTC(), method: r.Method, target: r.RequestURI}
		tc, _ := r.Context().Value(connKey{}).(*tapConn)
		if tc != nil {
			tc.begin(e)
		}
		t.checkCredentials(r)
		rw := &recorder{ResponseWriter: w, ex: e, requestID: t.opts.RequestID}
		if tc == nil {
			// Not served through Listener+Install: no raw files to point at,
			// but the request is still listed.
			defer func() {
				t.write(entry{Seq: e.seq, T: e.t, Method: e.method, Target: e.target, Status: e.getStatus()})
			}()
		}
		next.ServeHTTP(rw, r)
	})
}

// checkCredentials warns once when a request carries a credential that does
// not look fake. The files keep it verbatim (that is the point), so the
// operator needs to know a real secret is landing on disk.
func (t *Tap) checkCredentials(r *http.Request) {
	for _, name := range []string{"Authorization", "X-Api-Key"} {
		v := r.Header.Get(name)
		if v != "" && !strings.Contains(strings.ToLower(v), "fake") {
			t.warn(fmt.Sprintf("capture: a request carries a %s value without a \"fake\" marker; the capture stores it verbatim — use a fake token", name))
		}
	}
}

// recorder notes the status and optionally stamps request-id, forwarding
// everything else (streaming flushes, hijacks) to the real writer.
type recorder struct {
	http.ResponseWriter
	ex        *exchange
	requestID bool
	wrote     bool
}

func (w *recorder) WriteHeader(code int) {
	if !w.wrote {
		w.wrote = true
		// 1xx are informational; the final status is what the client got.
		if code >= 200 {
			w.ex.setStatus(code)
			if w.requestID && w.Header().Get("request-id") == "" {
				w.Header().Set("request-id", fmt.Sprintf("req_vmodel_%04d", w.ex.seq))
			}
		} else {
			w.wrote = false
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *recorder) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *recorder) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *recorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("capture: underlying ResponseWriter is not a Hijacker")
	}
	return h.Hijack()
}

func (w *recorder) CloseNotify() <-chan bool {
	if n, ok := w.ResponseWriter.(http.CloseNotifier); ok {
		return n.CloseNotify()
	}
	return make(chan bool)
}
