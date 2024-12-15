package middleware

import (
	"log"
	"net/http"
	"time"
)

type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.statusCode = statusCode
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/subscribe" {
			// dont include this route in middleware
			// because the wrapped one gives error
			// given ResponseWriter is not a http.Hijacker
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &wrappedWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		// eg. 200 GET /path in 150ms
		log.Printf("%v %v %v in %v\n", wrapped.statusCode, r.Method, r.URL.Path, time.Since(start))
	})
}
