package draupnir

// acquire reserves a concurrency slot without blocking. It returns false when
// the limiter is saturated so the caller can shed load (e.g. respond 503).
func (wp *WorkerPool) acquire() bool {
	select {
	case wp.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// release returns a previously acquired slot.
func (wp *WorkerPool) release() {
	<-wp.sem
}
