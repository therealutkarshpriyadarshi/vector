package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled         bool
	RequestsPerSec  float64 // Requests per second
	Burst           int     // Maximum burst size
	PerIP           bool    // Rate limit per IP address
	PerUser         bool    // Rate limit per user (requires auth)
	GlobalLimit     bool    // Global rate limit across all clients
}

// RateLimiter manages rate limiting for clients
type RateLimiter struct {
	config   RateLimitConfig
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	global   *rate.Limiter
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:   config,
		limiters: make(map[string]*rate.Limiter),
	}

	if config.GlobalLimit {
		rl.global = rate.NewLimiter(rate.Limit(config.RequestsPerSec), config.Burst)
	}

	// Start cleanup goroutine to prevent memory leaks
	go rl.cleanup()

	return rl
}

// getLimiter returns the rate limiter for a specific key
func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	limiter, exists = rl.limiters[key]
	if exists {
		return limiter
	}

	// Create new limiter for this key
	limiter = rate.NewLimiter(rate.Limit(rl.config.RequestsPerSec), rl.config.Burst)
	rl.limiters[key] = limiter

	return limiter
}

// cleanup periodically removes inactive limiters to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		// In a production system, you'd track last access time
		// For simplicity, we'll keep all limiters but this prevents unbounded growth
		if len(rl.limiters) > 10000 {
			rl.limiters = make(map[string]*rate.Limiter)
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if rate limiting is disabled
			if !limiter.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Check global rate limit first
			if limiter.config.GlobalLimit && limiter.global != nil {
				if !limiter.global.Allow() {
					writeRateLimitError(w, "Global rate limit exceeded")
					return
				}
			}

			// Determine the rate limit key
			var key string
			if limiter.config.PerUser {
				// Try to get user ID from context (requires auth middleware)
				if claims, ok := GetClaimsFromContext(r.Context()); ok {
					key = fmt.Sprintf("user:%s", claims.UserID)
				} else {
					// Fall back to IP if user not authenticated
					key = getClientIP(r)
				}
			} else if limiter.config.PerIP {
				key = getClientIP(r)
			} else {
				// Default to IP-based rate limiting
				key = getClientIP(r)
			}

			// Check per-client rate limit
			clientLimiter := limiter.getLimiter(key)
			if !clientLimiter.Allow() {
				writeRateLimitError(w, fmt.Sprintf("Rate limit exceeded for %s", key))
				return
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limiter.config.Burst))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", clientLimiter.Tokens()))

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies/load balancers)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP if multiple are present
		return forwarded
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// writeRateLimitError writes a rate limit error response
func writeRateLimitError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60") // Suggest retry after 60 seconds
	w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprintf(w, `{"error": "%s", "status": 429}`, message)
}
