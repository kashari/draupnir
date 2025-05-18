package draupnir

import "time"

func (rl *RateLimiter) refillTokens() {
	ticker := time.NewTicker(rl.refillInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			rl.tokens = rl.maxTokens
			rl.mu.Unlock()
		case <-rl.quit:
			return
		}
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

func (rl *RateLimiter) Stop() {
	close(rl.quit)
}
