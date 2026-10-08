package http

import (
	"encoding/json"
	"log"
	stdhttp "net/http"
	"time"

	"example.com/payments/pkg/id"
)

// statusRecorder wraps ResponseWriter to capture status code for logging.
type statusRecorder struct {
	stdhttp.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// withRequestIDAndLogging ensures X-Request-Id, adds it to response, and logs structured line per request.
func withRequestIDAndLogging(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = id.New()
		}

		w.Header().Set("X-Request-Id", reqID)

		rec := &statusRecorder{ResponseWriter: w, status: 200}
		start := time.Now()

		next.ServeHTTP(rec, r)

		entry := map[string]any{
			"ts":          time.Now().UTC().Format(time.RFC3339Nano),
			"level":       "info",
			"msg":         "http_request",
			"reqId":       reqID,
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      rec.status,
			"duration_ms": time.Since(start).Milliseconds(),
		}
		b, _ := json.Marshal(entry)
		log.Println(string(b))
	})
}
