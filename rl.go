package draupnir

import "time"

// refillTokens resets the bucket to its maximum on every interval.
func (rl *RateLimiter) refillTokens() {
	ticker := time.NewTicker(rl.refillInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.tokens.Store(rl.maxTokens)
		case <-rl.quit:
			return
		}
	}
}

// Allow consumes a token if one is available. It is safe for concurrent use and
// takes no locks on the hot path.
func (rl *RateLimiter) Allow() bool {
	for {
		v := rl.tokens.Load()
		if v <= 0 {
			return false
		}
		if rl.tokens.CompareAndSwap(v, v-1) {
			return true
		}
	}
}

// Stop terminates the refill goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.quit)
}
