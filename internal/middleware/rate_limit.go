package middleware

import (
	"net/http"
	"sync"
	"time"

	"go-server/internal/config"
	"go-server/pkg/utils"

	"github.com/gin-gonic/gin"
)

type visitor struct {
	limiter  *rateLimiter
	lastSeen time.Time
}

type rateLimiter struct {
	tokens    int
	maxTokens int
	refillRate time.Duration
	lastRefill time.Time
	mu        sync.Mutex
}

var (
	visitors = make(map[string]*visitor)
	mu       sync.RWMutex
)

func newRateLimiter(rps int) *rateLimiter {
	return &rateLimiter{
		tokens:     rps,
		maxTokens:  rps,
		refillRate: time.Second,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on time passed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	refills := int(elapsed / rl.refillRate)

	if refills > 0 {
		rl.tokens = min(rl.tokens+refills, rl.maxTokens)
		rl.lastRefill = now
	}

	// Check if we have tokens available
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RateLimit middleware implements token bucket rate limiting
func RateLimit(cfg *config.Config) gin.HandlerFunc {
	// Cleanup old visitors every 5 minutes
	go cleanupVisitors()

	return func(c *gin.Context) {
		if !cfg.RateLimitEnabled {
			c.Next()
			return
		}

		ip := c.ClientIP()

		mu.Lock()
		v, exists := visitors[ip]
		if !exists {
			v = &visitor{
				limiter:  newRateLimiter(cfg.RateLimitRPS),
				lastSeen: time.Now(),
			}
			visitors[ip] = v
		}
		v.lastSeen = time.Now()
		mu.Unlock()

		if !v.limiter.allow() {
			utils.ErrorResponse(
				c,
				http.StatusTooManyRequests,
				"RATE_LIMIT_EXCEEDED",
				"Too many requests. Please try again later.",
				nil,
			)
			c.Abort()
			return
		}

		c.Next()
	}
}

func cleanupVisitors() {
	for {
		time.Sleep(5 * time.Minute)

		mu.Lock()
		for ip, v := range visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(visitors, ip)
			}
		}
		mu.Unlock()
	}
}