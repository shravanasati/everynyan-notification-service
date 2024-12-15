package middleware

import (
	"net/http"
	"strings"
	"github.com/shravanasati/everynyan-notification-service/config"
)

var config_ = config.MustConfig()

func EnsureAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		splittedAuth := strings.Split(auth, " ")
		if len(splittedAuth) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing authorization"))
			return
		}

		apiKey := splittedAuth[1]
		if apiKey != config_.API_KEY {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("invalid api key"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
