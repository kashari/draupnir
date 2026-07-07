package draupnir

import (
	"fmt"
	"net/http"
	"os"
	"sync"
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
		golog.Error("WebSocket upgrade error for {}: {} (status {})", req.URL.Path, reason, status)
		http.Error(w, reason.Error(), status)
	},
}

// DefaultLogFile is the file the router initializes golog with when the
// application never calls WithFileLogging (or golog.Init) itself. golog panics
// if used before initialization, so the router installs this default lazily.
var DefaultLogFile = "draupnir.log"

var loggingInitOnce sync.Once

// ensureLogging guarantees golog is usable. If logging was configured on the
// router (WithFileLogging), nothing happens. Otherwise it probes golog: golog
// panics when uninitialized, so a recovered panic means we must install the
// default sink; no panic means logging was initialized externally and is left
// untouched.
func (r *Router) ensureLogging() {
	if r.loggingConfigured {
		return
	}
	loggingInitOnce.Do(func() {
		defer func() {
			if recover() != nil {
				if err := golog.Init(DefaultLogFile); err != nil {
					fmt.Fprintf(os.Stderr, "draupnir: failed to initialize default logging: %v\n", err)
				}
			}
		}()
		golog.Debug("draupnir logging initialized")
	})
}

// NewRateLimiter creates a new fixed-window rate limiter.
func NewRateLimiter(maxTokens int, refillInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		maxTokens:      int64(maxTokens),
		refillInterval: refillInterval,
		quit:           make(chan struct{}),
	}
	rl.tokens.Store(int64(maxTokens))
	go rl.refillTokens()
	return rl
}

func New() *Router {
	r := &Router{
		staticRoutes:  tree.New(),
		dynamicRoutes: make([]route, 0),
		middlewares:   []HandlerFunc{},
	}
	return r
}

// NewWorkerPool creates a concurrency limiter that caps the number of
// simultaneously executing handlers at size.
func NewWorkerPool(size int) *WorkerPool {
	return &WorkerPool{
		sem:  make(chan struct{}, size),
		size: size,
	}
}
