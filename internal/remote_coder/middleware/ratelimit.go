package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	attempts      map[string][]time.Time // IP -> list of attempt times
	blockUntil    map[string]time.Time   // IP -> unblock time
	mu            sync.RWMutex
	maxAttempts   int
	windowSize    time.Duration
	blockDuration time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxAttempts int, windowSize, blockDuration time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts:      make(map[string][]time.Time),
		blockUntil:    make(map[string]time.Time),
		maxAttempts:   maxAttempts,
		windowSize:    windowSize,
		blockDuration: blockDuration,
	}
}

// isBlocked checks if an IP is currently blocked
func (rl *RateLimiter) isBlocked(ip string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if unblockAt, exists := rl.blockUntil[ip]; exists {
		if time.Now().Before(unblockAt) {
			return true
		}
		// Block expired, clean up
		delete(rl.blockUntil, ip)
	}
	return false
}

// recordAttempt records an authentication attempt
func (rl *RateLimiter) recordAttempt(ip string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Clean old attempts outside the window
	cutoff := now.Add(-rl.windowSize)
	attempts := rl.attempts[ip]
	var recentAttempts []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			recentAttempts = append(recentAttempts, t)
		}
	}
	recentAttempts = append(recentAttempts, now)
	rl.attempts[ip] = recentAttempts

	// Check if should block
	if len(recentAttempts) >= rl.maxAttempts {
		rl.blockUntil[ip] = now.Add(rl.blockDuration)
		logrus.Warnf("IP %s blocked for %v due to too many failed attempts", ip, rl.blockDuration)
		return false
	}

	return true
}

// getRemainingAttempts calculates remaining attempts before block
func (rl *RateLimiter) getRemainingAttempts(ip string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	cutoff := time.Now().Add(-rl.windowSize)
	count := 0
	for _, t := range rl.attempts[ip] {
		if t.After(cutoff) {
			count++
		}
	}
	remaining := rl.maxAttempts - count
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// cleanup runs periodically to remove expired entries
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.windowSize - rl.blockDuration)

	// Clean old attempts
	for ip, attempts := range rl.attempts {
		var recentAttempts []time.Time
		for _, t := range attempts {
			if t.After(cutoff) {
				recentAttempts = append(recentAttempts, t)
			}
		}
		if len(recentAttempts) == 0 {
			delete(rl.attempts, ip)
		} else {
			rl.attempts[ip] = recentAttempts
		}
	}

	// Clean expired blocks
	for ip, unblockAt := range rl.blockUntil {
		if now.After(unblockAt) {
			delete(rl.blockUntil, ip)
		}
	}
}

// RateLimitMiddleware returns a Gin middleware for rate limiting
// This is specifically for auth endpoints (handshake, execute)
func RateLimitMiddleware(rl *RateLimiter, authPaths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		// Check if blocked
		if rl.isBlocked(ip) {
			c.Header("Retry-After", "300")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message":     "Too many failed attempts. Please try again in 5 minutes.",
					"type":        "rate_limit_error",
					"retry_after": 300,
				},
			})
			return
		}

		// Track the attempt for auth endpoints
		path := c.Request.URL.Path
		isAuthPath := false
		for _, authPath := range authPaths {
			if path == authPath {
				isAuthPath = true
				break
			}
		}

		if isAuthPath && c.Request.Method == "POST" {
			if !rl.recordAttempt(ip) {
				c.Header("Retry-After", "300")
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"message":     "Too many failed attempts. Please try again in 5 minutes.",
						"type":        "rate_limit_error",
						"retry_after": 300,
					},
				})
				return
			}
		}

		c.Next()
	}
}

// ResetIP resets the rate limit for a specific IP (admin use only)
func (rl *RateLimiter) ResetIP(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
	delete(rl.blockUntil, ip)
	logrus.Infof("Rate limit reset for IP: %s", ip)
}

// GetStats returns rate limiting statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	blockedCount := 0
	for _, t := range rl.blockUntil {
		if time.Now().Before(t) {
			blockedCount++
		}
	}

	return map[string]interface{}{
		"total_ips_tracked": len(rl.attempts),
		"currently_blocked": blockedCount,
		"max_attempts":      rl.maxAttempts,
		"window_size":       rl.windowSize.String(),
		"block_duration":    rl.blockDuration.String(),
	}
}
