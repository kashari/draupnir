package draupnir

import (
	"context"
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
	middlewares []Middleware
	router      *Router
}

// Use adds a middleware to the chain.
func (r *Router) Use(m Middleware) *Router {
	r.middlewares = append(r.middlewares, m)
	return r
}

// Group creates a new route group with the specified prefix.
// All routes registered on this group will be prefixed with the given prefix.
func (r *Router) Group(prefix string) *RouterGroup {
	return &RouterGroup{
		prefix:      prefix,
		middlewares: make([]Middleware, 0),
		router:      r,
	}
}

// Use adds middleware to the route group.
// This middleware will be applied to all routes in this group.
func (rg *RouterGroup) Use(m Middleware) *RouterGroup {
	rg.middlewares = append(rg.middlewares, m)
	return rg
}

// Group creates a sub-group with an additional prefix.
// The new group will inherit the current group's prefix and middleware.
func (rg *RouterGroup) Group(prefix string) *RouterGroup {
	return &RouterGroup{
		prefix:      rg.prefix + prefix,
		middlewares: append([]Middleware{}, rg.middlewares...), // Copy middlewares
		router:      rg.router,
	}
}

// HandleFunc registers a route using a Context-based handler in the group.
func (rg *RouterGroup) HandleFunc(method, pattern string, handler func(*Context)) *RouterGroup {
	fullPattern := rg.prefix + pattern

	// Create a handler that applies group middlewares
	wrappedHandler := func(w http.ResponseWriter, req *http.Request) {
		ctx := &Context{Writer: w, Request: req}

		// Create a handler function that applies group middlewares
		finalHandler := func(c *Context) {
			handler(c)
		}

		// Apply group middlewares in reverse order
		for i := len(rg.middlewares) - 1; i >= 0; i-- {
			middleware := rg.middlewares[i]
			currentHandler := finalHandler
			finalHandler = func(c *Context) {
				// Convert Context-based handler to http.HandlerFunc for middleware
				httpHandler := func(w http.ResponseWriter, r *http.Request) {
					currentHandler(c)
				}
				// Apply middleware and convert back
				wrappedHttpHandler := middleware(httpHandler)
				wrappedHttpHandler(w, req)
			}
		}

		finalHandler(ctx)
	}

	rt := route{
		method:  method,
		pattern: fullPattern,
		handler: wrappedHandler,
	}

	if !strings.ContainsAny(fullPattern, ":*") {
		rg.router.staticRoutes.Insert(fullPattern, rt)
	} else {
		rg.router.dynamicRoutes = append(rg.router.dynamicRoutes, rt)
	}
	return rg
}

// HTTP method helpers for RouterGroup
func (rg *RouterGroup) GET(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc("GET", pattern, handler)
}

func (rg *RouterGroup) POST(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodPost, pattern, handler)
}

func (rg *RouterGroup) PUT(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodPut, pattern, handler)
}

func (rg *RouterGroup) DELETE(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodDelete, pattern, handler)
}

func (rg *RouterGroup) PATCH(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodPatch, pattern, handler)
}

func (rg *RouterGroup) OPTIONS(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodOptions, pattern, handler)
}

func (rg *RouterGroup) HEAD(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodHead, pattern, handler)
}

func (rg *RouterGroup) TRACE(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodTrace, pattern, handler)
}

func (rg *RouterGroup) CONNECT(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodConnect, pattern, handler)
}

func (rg *RouterGroup) ANY(pattern string, handler func(*Context)) *RouterGroup {
	return rg.HandleFunc(http.MethodGet, pattern, handler)
}

// WithWorkerPool configures the router to use a worker pool.
func (r *Router) WithWorkerPool(poolSize int) *Router {
	r.workerPool = NewWorkerPool(poolSize)
	return r
}

// WithRateLimiter configures the router to use a rate limiter.
func (r *Router) WithRateLimiter(maxTokens int, refillInterval time.Duration) *Router {
	r.rateLimiter = NewRateLimiter(maxTokens, refillInterval)
	return r
}

// WithFileLogging configures the router to log to the specified file in addition to the console.
// If the file cannot be opened, it logs an error and leaves the existing logger intact.
func (r *Router) WithFileLogging(filePath string) *Router {
	err := golog.Init(filePath)
	if err != nil {
		golog.Error("Failed to open log file {}: {}}", filePath, err)
	} else {
		golog.Info("Logging to file {}", filePath)
	}

	return r
}

// Handle registers a new route.
func (r *Router) Handle(method, pattern string, handler http.HandlerFunc) *Router {
	rt := route{
		method:  method,
		pattern: pattern,
		handler: handler,
	}
	if !strings.ContainsAny(pattern, ":*") {
		r.staticRoutes.Insert(pattern, rt)
	} else {
		r.dynamicRoutes = append(r.dynamicRoutes, rt)
	}
	return r
}

// HandleFunc registers a route using a Context-based handler.
func (r *Router) HandleFunc(method, pattern string, handler func(*Context)) *Router {
	rt := route{
		method:  method,
		pattern: pattern,
		handler: func(w http.ResponseWriter, req *http.Request) {
			ctx := &Context{Writer: w, Request: req}
			handler(ctx)
		},
	}
	if !strings.ContainsAny(pattern, ":*") {
		r.staticRoutes.Insert(pattern, rt)
	} else {
		r.dynamicRoutes = append(r.dynamicRoutes, rt)
	}
	return r
}

func (r *Router) GET(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc("GET", pattern, handler)
}

func (r *Router) POST(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodPost, pattern, handler)
}

func (r *Router) PUT(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodPut, pattern, handler)
}

func (r *Router) DELETE(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodDelete, pattern, handler)
}

func (r *Router) PATCH(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodPatch, pattern, handler)
}

func (r *Router) OPTIONS(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodOptions, pattern, handler)
}

func (r *Router) HEAD(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodHead, pattern, handler)
}

func (r *Router) TRACE(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodTrace, pattern, handler)
}

func (r *Router) CONNECT(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodConnect, pattern, handler)
}

func (r *Router) ANY(pattern string, handler func(*Context)) *Router {
	return r.HandleFunc(http.MethodGet, pattern, handler)
}

// ListRoutes returns a slice of strings describing all registered routes.
func (r *Router) ListRoutes() []string {
	var routes []string
	r.staticRoutes.Walk(func(path string, v interface{}) bool {
		rt := v.(route)
		routes = append(routes, rt.method+" "+rt.pattern)
		return false
	})
	for _, rt := range r.dynamicRoutes {
		routes = append(routes, rt.method+" "+rt.pattern)
	}
	return routes
}

// ServeHTTP implements http.Handler.
// It checks if the request matches a static or dynamic route and executes the corresponding handler.
// If no route matches, it returns a 404 Not Found error.
// It also logs the request details and execution time.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	if val, found := r.staticRoutes.Get(req.URL.Path); found {
		rt := val.(route)
		if rt.method == req.Method || req.Method == http.MethodOptions {
			r.executeHandler(w, req, rt.handler)
			golog.Debug("(STATIC ROUTE) Request: {} {}, from: {} completed in {}", req.Method, req.URL.Path, req.RemoteAddr, time.Since(start))
			return
		}

		w.Header().Set("Allow", rt.method)
		http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		golog.Warn("Method not allowed (static) {}", time.Since(start).String())
		return
	}

	for _, rt := range r.dynamicRoutes {
		if params, ok := matchPattern(rt.pattern, req.URL.Path); ok && (rt.method == req.Method || req.Method == http.MethodOptions) {
			ctx := context.WithValue(req.Context(), paramsKey, params)
			r.executeHandler(w, req.WithContext(ctx), rt.handler)
			golog.Debug("(DYNAMIC ROUTE) Request: {} {}, from: {} completed in {}", req.Method, req.URL.Path, req.RemoteAddr, time.Since(start))
			return
		}
	}

	http.NotFound(w, req)
	golog.Warn("Route not found {}", time.Since(start).String())
}

// executeHandler runs the handler with the middleware chain and rate limiter.
func (r *Router) executeHandler(w http.ResponseWriter, req *http.Request, handler http.HandlerFunc) {
	finalHandler := handler
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		finalHandler = r.middlewares[i](finalHandler)
	}

	if r.rateLimiter != nil && !r.rateLimiter.Allow() {
		http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
		return
	}

	if r.workerPool != nil {
		done := make(chan struct{})
		err := r.workerPool.Submit(func() {
			finalHandler(w, req)
			close(done)
		})
		if err != nil {
			http.Error(w, "503 Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		<-done // wait for completion
	} else {
		finalHandler(w, req)
	}
}

// Start launches the HTTP server on the specified port after printing full configuration.
// It also logs the startup information and registered routes.
// The server will listen on the specified port and handle incoming requests.
// The server will run indefinitely until an error occurs or the program is terminated.
//
// Parameters:
//   - port: The port on which the server will listen for incoming requests.
// Returns an error if the server fails to start or encounters an issue during execution
// Example usage:
//   err := router.Start("8080")
//   if err != nil {
//       log.Fatal(err)
//   }

// Note: The server will block the calling goroutine until it is stopped or an error occurs.
//
//	Ensure to handle graceful shutdowns and cleanup as needed.
func (r *Router) Start(port string) error {
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
	// Log registered routes.
	golog.Info("-------------------------- Registered Routes ---------------------------")
	for _, rt := range r.ListRoutes() {
		golog.Info("Route " + rt)
	}
	golog.Info("-------------------------- Registered Routes ---------------------------")

	// Rate limiter configuration.
	if r.rateLimiter != nil {
		golog.Info("Rate Limiter Configuration MAX_TOKENS: {} REFILL_INTERVAL: {}", r.rateLimiter.maxTokens, r.rateLimiter.refillInterval)
	} else {
		golog.Info("Rate Limiter not configured")
	}
	// Worker pool configuration.
	if r.workerPool != nil {
		golog.Info("Worker Pool Configuration SIZE: {}", r.workerPool.size)
	} else {
		golog.Info("Worker Pool not configured")
	}

	if len(r.middlewares) > 0 {
		golog.Info("-------------------------- Middleware Chain ---------------------------")
		golog.Info("--")
		for i, mw := range r.middlewares {
			golog.Info("Middleware {}: {}", i, getFunctionName(mw))
		}
		golog.Info("--")
		golog.Info("-------------------------- Middleware Chain ---------------------------")
	}

}

// matchPattern compares a route pattern with a request path.
func matchPattern(pattern, path string) (map[string]string, bool) {
	patternParts := splitPath(pattern)
	pathParts := splitPath(path)
	if len(patternParts) != len(pathParts) {
		return nil, false
	}
	params := make(map[string]string)
	for i, part := range patternParts {
		if len(part) > 0 && part[0] == ':' {
			key := part[1:]
			params[key] = pathParts[i]
		} else if part != pathParts[i] {
			return nil, false
		}
	}
	return params, true
}

// splitPath splits a URL path into non-empty segments.
func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
}

// getFunctionName returns the name of a function (used for middleware identification).
func getFunctionName(i any) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
