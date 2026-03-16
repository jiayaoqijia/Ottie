package middleware

import (
	"net"
	"net/http"

	"github.com/jiayaoqijia/ottie/pkg/gateway"
)

// RateLimit returns middleware that rate-limits requests using the given
// RateLimiter and scope. Requests that exceed the limit receive a 429 response.
func RateLimit(limiter *gateway.RateLimiter, scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			if !limiter.Allow(scope, ip) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
