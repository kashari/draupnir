package draupnir

import (
	"net/http"
	"time"

	"github.com/kashari/draupnir/tree"
	"github.com/kashari/draupnir/ws"
	"github.com/kashari/golog"
)

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(req *http.Request) bool {
		// IMPORTANT: Implement proper origin checking for security.
		// This default allows all origins, which might be insecure.
		// Example:
		// allowedOrigins := []string{"http://localhost:3000", "https://yourdomain.com"}
		// origin := req.Header.Get("Origin")
		// for _, allowed := range allowedOrigins {
		//    if origin == allowed {
		//        return true
		//    }
		// }
		// return false
		return true // Placeholder: Allow all origins
	},
	HandshakeTimeout: 10 * time.Second,
	Error: func(w http.ResponseWriter, req *http.Request, status int, reason error) {
		// Custom error logging or response
		// For example, log the error:
		// log.Printf("WebSocket upgrade error for %s: %v (status %d)", req.URL.Path, reason, status)
		golog.Error("WebSocket upgrade error for {}: {} (status {})", req.URL.Path, reason, status)
		http.Error(w, reason.Error(), status)
	},
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(maxTokens int, refillInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		tokens:         maxTokens,
		maxTokens:      maxTokens,
		refillInterval: refillInterval,
		quit:           make(chan struct{}),
	}
	go rl.refillTokens()
	return rl
}

func New() *Router {
	r := &Router{
		staticRoutes:  tree.New(),
		dynamicRoutes: make([]route, 0),
		middlewares:   []Middleware{},
	}
	return r
}

// NewWorkerPool creates a new worker pool with the given size.
// It sets the channel buffer to size*10 to allow bursts of tasks.
func NewWorkerPool(size int) *WorkerPool {
	wp := &WorkerPool{
		tasks: make(chan func(), size*10),
		size:  size,
	}
	for range size {
		go wp.worker()
	}
	return wp
}
