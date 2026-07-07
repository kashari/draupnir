package draupnir

import (
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/kashari/golog"
)

// RouterGroup represents a group of routes with a common prefix and middleware.
type RouterGroup struct {
	prefix      string
	middlewares []HandlerFunc
	router      *Router
}

// Use adds one or more Context-native middleware to the global chain.
//
// Middleware are composed into each route's chain at registration time, so
// Use must be called before the routes it should apply to (gin-style).
func (r *Router) Use(m ...HandlerFunc) *Router {
	r.middlewares = append(r.middlewares, m...)
	return r
}

// UseHTTP attaches a classic net/http middleware (func(http.HandlerFunc) http.HandlerFunc).
//
// The wrapped middleware should call next to continue the chain; if it does not,
// the request is aborted and downstream handlers are skipped.
func (r *Router) UseHTTP(m Middleware) *Router {
	return r.Use(wrapHTTPMiddleware(m))
}

// wrapHTTPMiddleware adapts a net/http middleware into a Context-native one.
func wrapHTTPMiddleware(m Middleware) HandlerFunc {
	return func(c *Context) {
		called := false
		next := func(http.ResponseWriter, *http.Request) {
			called = true
			c.Next()
		}
		m(next)(c.Writer, c.Request)
		if !called {
			c.Abort()
		}
	}
}

// Group creates a new route group with the specified prefix.
func (r *Router) Group(prefix string) *RouterGroup {
	return &RouterGroup{
		prefix:      prefix,
		middlewares: make([]HandlerFunc, 0),
		router:      r,
	}
}

// Use adds middleware applied to all routes registered on this group afterwards.
func (rg *RouterGroup) Use(m ...HandlerFunc) *RouterGroup {
	rg.middlewares = append(rg.middlewares, m...)
	return rg
}

// Group creates a sub-group that inherits this group's prefix and middleware.
func (rg *RouterGroup) Group(prefix string) *RouterGroup {
	return &RouterGroup{
		prefix:      rg.prefix + prefix,
		middlewares: append([]HandlerFunc{}, rg.middlewares...),
		router:      rg.router,
	}
}

// HandleFunc registers a Context-based handler on the group.
func (rg *RouterGroup) HandleFunc(method, pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	chain := rg.router.buildChain(rg.middlewares, mw, handler)
	rg.router.register(method, rg.prefix+pattern, false, chain)
	return rg
}

// HTTP method helpers for RouterGroup
func (rg *RouterGroup) GET(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(GET, pattern, handler, mw...)
}

func (rg *RouterGroup) POST(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodPost, pattern, handler, mw...)
}

func (rg *RouterGroup) PUT(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodPut, pattern, handler, mw...)
}

func (rg *RouterGroup) DELETE(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodDelete, pattern, handler, mw...)
}

func (rg *RouterGroup) PATCH(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodPatch, pattern, handler, mw...)
}

func (rg *RouterGroup) OPTIONS(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodOptions, pattern, handler, mw...)
}

func (rg *RouterGroup) HEAD(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodHead, pattern, handler, mw...)
}

func (rg *RouterGroup) TRACE(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodTrace, pattern, handler, mw...)
}

func (rg *RouterGroup) CONNECT(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodConnect, pattern, handler, mw...)
}

func (rg *RouterGroup) ANY(pattern string, handler HandlerFunc, mw ...HandlerFunc) *RouterGroup {
	return rg.HandleFunc(http.MethodGet, pattern, handler, mw...)
}

// WithWorkerPool caps the number of concurrently executing handlers.
func (r *Router) WithWorkerPool(poolSize int) *Router {
	r.workerPool = NewWorkerPool(poolSize)
	return r
}

// WithRateLimiter configures the router to use a rate limiter.
func (r *Router) WithRateLimiter(maxTokens int, refillInterval time.Duration) *Router {
	r.rateLimiter = NewRateLimiter(maxTokens, refillInterval)
	return r
}

// WithFileLogging configures the router to log to the specified file in addition
// to the console. It also marks logging as externally configured so the router
// does not install its default log sink.
func (r *Router) WithFileLogging(filePath string) *Router {
	r.loggingConfigured = true
	if err := golog.Init(filePath); err != nil {
		golog.Error("Failed to open log file {}: {}", filePath, err)
	} else {
		golog.Info("Logging to file {}", filePath)
	}
	return r
}

// buildChain composes global + group + per-route middleware and the leaf handler
// into a single slice, once, at registration time.
func (r *Router) buildChain(groupMW, routeMW []HandlerFunc, h HandlerFunc) []HandlerFunc {
	chain := make([]HandlerFunc, 0, len(r.middlewares)+len(groupMW)+len(routeMW)+1)
	chain = append(chain, r.middlewares...)
	chain = append(chain, groupMW...)
	chain = append(chain, routeMW...)
	chain = append(chain, h)
	return chain
}

// register stores a compiled route in either the static tree or the dynamic slice.
func (r *Router) register(method, pattern string, streaming bool, chain []HandlerFunc) *Router {
	rt := route{
		method:    method,
		pattern:   pattern,
		handlers:  chain,
		streaming: streaming,
	}
	if strings.ContainsAny(pattern, ":*") {
		rt.segments, rt.wildcard = compilePattern(pattern)
		r.dynamicRoutes = append(r.dynamicRoutes, rt)
	} else {
		r.insertStatic(pattern, rt)
	}
	return r
}

// insertStatic adds a static route, allowing multiple methods per path.
func (r *Router) insertStatic(pattern string, rt route) {
	if v, ok := r.staticRoutes.Get(pattern); ok {
		v.(*staticEntry).methods[rt.method] = rt
		return
	}
	entry := &staticEntry{methods: map[string]route{rt.method: rt}}
	r.staticRoutes.Insert(pattern, entry)
}

// Handle registers a classic net/http handler.
func (r *Router) Handle(method, pattern string, handler http.HandlerFunc) *Router {
	return r.HandleFunc(method, pattern, func(c *Context) {
		handler(c.Writer, c.Request)
	})
}

// HandleFunc registers a route using a Context-based handler.
func (r *Router) HandleFunc(method, pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.register(method, pattern, false, r.buildChain(nil, mw, handler))
}

// handleStreaming registers a long-lived (WebSocket/SSE) route that bypasses the
// rate limiter and concurrency limiter.
func (r *Router) handleStreaming(method, pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.register(method, pattern, true, r.buildChain(nil, mw, handler))
}

func (r *Router) GET(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(GET, pattern, handler, mw...)
}

func (r *Router) POST(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodPost, pattern, handler, mw...)
}

func (r *Router) PUT(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodPut, pattern, handler, mw...)
}

func (r *Router) DELETE(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodDelete, pattern, handler, mw...)
}

func (r *Router) PATCH(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodPatch, pattern, handler, mw...)
}

func (r *Router) OPTIONS(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodOptions, pattern, handler, mw...)
}

func (r *Router) HEAD(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodHead, pattern, handler, mw...)
}

func (r *Router) TRACE(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodTrace, pattern, handler, mw...)
}

func (r *Router) CONNECT(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodConnect, pattern, handler, mw...)
}

func (r *Router) ANY(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router {
	return r.HandleFunc(http.MethodGet, pattern, handler, mw...)
}

// ListRoutes returns a slice of strings describing all registered routes.
func (r *Router) ListRoutes() []string {
	var routes []string
	r.staticRoutes.Walk(func(path string, v interface{}) bool {
		for m := range v.(*staticEntry).methods {
			routes = append(routes, m+" "+path)
		}
		return false
	})
	for _, rt := range r.dynamicRoutes {
		routes = append(routes, rt.method+" "+rt.pattern)
	}
	return routes
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.ensureLogging()

	c := acquireContext()
	c.reset(w, req, r)
	defer releaseContext(c)

	path := req.URL.Path

	// Static routes first (exact-path radix lookup).
	if v, found := r.staticRoutes.Get(path); found {
		entry := v.(*staticEntry)
		if rt, ok := entry.methods[req.Method]; ok {
			r.runChain(c, &rt)
			return
		}
		if req.Method == OPTIONS {
			for _, rt := range entry.methods {
				r.runChain(c, &rt)
				return
			}
		}
		w.Header().Set("Allow", allowHeader(entry))
		http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Dynamic routes: split the path once, then scan.
	if len(r.dynamicRoutes) > 0 {
		parts := splitPath(path)
		for i := range r.dynamicRoutes {
			rt := &r.dynamicRoutes[i]
			if rt.method != req.Method && req.Method != OPTIONS {
				continue
			}
			c.params = c.params[:0]
			if rt.matchParts(parts, &c.params) {
				r.runChain(c, rt)
				return
			}
		}
	}

	http.NotFound(w, req)
	golog.Warn("Route not found {}", path)
}

// runChain applies the rate limiter and concurrency limiter (unless streaming),
// then executes the route's precompiled handler chain.
func (r *Router) runChain(c *Context, rt *route) {
	c.handlers = rt.handlers
	c.index = -1

	if rt.streaming {
		c.Next()
		return
	}

	if r.rateLimiter != nil && !r.rateLimiter.Allow() {
		http.Error(c.Writer, "429 Too Many Requests", http.StatusTooManyRequests)
		return
	}

	if r.workerPool != nil {
		if !r.workerPool.acquire() {
			http.Error(c.Writer, "503 Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		defer r.workerPool.release()
	}

	c.Next()
}

// Start launches the HTTP server on the specified port after printing configuration.
func (r *Router) Start(port string) error {
	r.ensureLogging()
	r.printStartupInfo()
	r.printConfiguration()
	golog.Info("Starting server in port {}", port)
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		IdleTimeout:  90 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}

func (r *Router) printStartupInfo() {
	logo := `
    .___                                 .__
  __| _/___________   __ ________   ____ |__|______
 / __ |\_  __ \__  \ |  |  \____ \ /    \|  \_  __ \
/ /_/ | |  | \// __ \|  |  /  |_> >   |  \  ||  | \/
\____ | |__|  (____  /____/|   __/|___|  /__||__|
     \/            \/      |__|        \/

			[ˈdrɔupnez̠] - A simple HTTP router for Go
`
	golog.Debug("{}", logo)
}

// printConfiguration logs all startup configuration details.
func (r *Router) printConfiguration() {
	golog.Info("-------------------------- Registered Routes ---------------------------")
	for _, rt := range r.ListRoutes() {
		golog.Info("Route " + rt)
	}
	golog.Info("-------------------------- Registered Routes ---------------------------")

	if r.rateLimiter != nil {
		golog.Info("Rate Limiter Configuration MAX_TOKENS: {} REFILL_INTERVAL: {}", r.rateLimiter.maxTokens, r.rateLimiter.refillInterval)
	} else {
		golog.Info("Rate Limiter not configured")
	}
	if r.workerPool != nil {
		golog.Info("Concurrency Limiter Configuration SIZE: {}", r.workerPool.size)
	} else {
		golog.Info("Concurrency Limiter not configured")
	}

	if len(r.middlewares) > 0 {
		golog.Info("-------------------------- Middleware Chain ---------------------------")
		for i, mw := range r.middlewares {
			golog.Info("Middleware {}: {}", i, getFunctionName(mw))
		}
		golog.Info("-------------------------- Middleware Chain ---------------------------")
	}
}

// compilePattern splits a dynamic pattern into precomputed segments.
func compilePattern(pattern string) (segs []segment, wildcard bool) {
	parts := splitPath(pattern)
	segs = make([]segment, len(parts))
	for i, p := range parts {
		switch {
		case len(p) > 0 && p[0] == ':':
			segs[i] = segment{kind: segParam, value: p[1:]}
		case len(p) > 0 && p[0] == '*':
			segs[i] = segment{kind: segWildcard, value: p[1:]}
			wildcard = true
		default:
			segs[i] = segment{kind: segLiteral, value: p}
		}
	}
	return segs, wildcard
}

// matchParts matches a request path (already split into parts) against the
// route's compiled segments, appending captured params.
func (rt *route) matchParts(parts []string, params *[]Param) bool {
	if rt.wildcard {
		fixed := len(rt.segments) - 1 // segments before the trailing wildcard
		if len(parts) < fixed {
			return false
		}
		for i := 0; i < fixed; i++ {
			seg := rt.segments[i]
			if seg.kind == segLiteral {
				if parts[i] != seg.value {
					return false
				}
			} else {
				*params = append(*params, Param{Key: seg.value, Value: parts[i]})
			}
		}
		rest := strings.Join(parts[fixed:], "/")
		*params = append(*params, Param{Key: rt.segments[fixed].value, Value: rest})
		return true
	}

	if len(parts) != len(rt.segments) {
		return false
	}
	for i, seg := range rt.segments {
		if seg.kind == segLiteral {
			if parts[i] != seg.value {
				return false
			}
		} else {
			*params = append(*params, Param{Key: seg.value, Value: parts[i]})
		}
	}
	return true
}

// allowHeader builds the Allow header value for a static path's methods.
func allowHeader(e *staticEntry) string {
	methods := make([]string, 0, len(e.methods))
	for m := range e.methods {
		methods = append(methods, m)
	}
	return strings.Join(methods, ", ")
}

// splitPath splits a URL path into non-empty segments.
func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
}

// getFunctionName returns the name of a function (used for middleware identification).
func getFunctionName(i any) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
