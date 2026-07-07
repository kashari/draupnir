package draupnir

import (
	"mime/multipart"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/kashari/draupnir/tree"
)

// Common HTTP methods
const (
	GET     = "GET"
	POST    = "POST"
	PUT     = "PUT"
	DELETE  = "DELETE"
	PATCH   = "PATCH"
	OPTIONS = "OPTIONS"
	HEAD    = "HEAD"
	TRACE   = "TRACE"
	CONNECT = "CONNECT"
)

// Common HTTP header fields
const (
	HeaderContentType        = "Content-Type"
	HeaderContentLength      = "Content-Length"
	HeaderContentDisposition = "Content-Disposition"
	HeaderAccept             = "Accept"
	HeaderAuthorization      = "Authorization"
	HeaderCacheControl       = "Cache-Control"
	HeaderConnection         = "Connection"
	HeaderTransferEncoding   = "Transfer-Encoding"
	HeaderXForwardedFor      = "X-Forwarded-For"
	HeaderUserAgent          = "User-Agent"
)

// Common content types
const (
	MIMEApplicationJSON        = "application/json"
	MIMEApplicationXML         = "application/xml"
	MIMEApplicationForm        = "application/x-www-form-urlencoded"
	MIMEMultipartForm          = "multipart/form-data"
	MIMETextPlain              = "text/plain"
	MIMETextHTML               = "text/html"
	MIMETextEventStream        = "text/event-stream"
	MIMEApplicationOctetStream = "application/octet-stream"
)

// HandlerFunc is the Context-native handler and middleware signature. Middleware
// call Context.Next to run the rest of the chain; a leaf handler simply returns.
type HandlerFunc func(*Context)

// MiddlewareFunc is an alias for HandlerFunc kept for readability at call sites.
type MiddlewareFunc = HandlerFunc

// Middleware is the classic net/http middleware signature. It is preserved for
// interop and can be attached with Router.UseHTTP.
type Middleware func(http.HandlerFunc) http.HandlerFunc

// segKind classifies a single compiled path segment.
type segKind uint8

const (
	segLiteral segKind = iota
	segParam
	segWildcard
)

// segment is one compiled component of a dynamic route pattern.
type segment struct {
	kind  segKind
	value string // literal text, or the capture name for params/wildcards
}

// route holds a registered endpoint with its precompiled handler chain.
type route struct {
	method    string
	pattern   string    // e.g., "/users/:id"
	segments  []segment // precompiled segments (dynamic routes only)
	wildcard  bool      // pattern ends with a *capture segment
	handlers  []HandlerFunc
	streaming bool // long-lived (WebSocket/SSE): bypass rate limiter + concurrency limit
}

// staticEntry stores every method registered for a single static path so that,
// for example, GET /x and POST /x can coexist.
type staticEntry struct {
	methods map[string]route
}

// WorkerPool caps the number of concurrently executing handlers using a
// buffered-channel semaphore. It adds no goroutines of its own; net/http already
// serves each request on its own goroutine.
type WorkerPool struct {
	sem  chan struct{}
	size int
}

// RateLimiter implements a fixed-window token limiter with a lock-free hot path.
type RateLimiter struct {
	tokens         atomic.Int64
	maxTokens      int64
	refillInterval time.Duration
	quit           chan struct{}
}

// Router is our HTTP router with integrated logging.
type Router struct {
	staticRoutes      *tree.Tree    // static routes stored by exact path -> *staticEntry
	dynamicRoutes     []route       // routes with parameters (e.g., ":id", "*path")
	middlewares       []HandlerFunc // global middleware chain
	workerPool        *WorkerPool   // optional concurrency limiter
	rateLimiter       *RateLimiter  // optional rate limiter on the critical path
	loggingConfigured bool          // true once WithFileLogging ran
}

// Param represents a single URL parameter
type Param struct {
	Key   string
	Value string
}

// Context represents the context for the current HTTP request
type Context struct {
	Request         *http.Request
	path            string
	method          string
	handlers        []HandlerFunc
	index           int
	aborted         bool
	params          []Param
	store           map[string]any
	Writer          http.ResponseWriter
	router          *Router
	bodyParsed      bool
	queryParsed     bool
	formParsed      bool
	multipartParsed bool
	query           url.Values
	formValues      url.Values
	multipartForm   *multipart.Form
	statusCode      int
}
