package draupnir

import (
	"mime/multipart"
	"net/http"
	"net/url"
	"sync"
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
	MIMEApplicationOctetStream = "application/octet-stream"
)

type ctxKey string

const paramsKey ctxKey = "params"

type route struct {
	method  string
	pattern string // e.g., "/users/:id"
	handler http.HandlerFunc
}

// Wrapper for http.HandlerFunc
type Middleware func(http.HandlerFunc) http.HandlerFunc

type WorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
	size  int
}

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	tokens         int
	maxTokens      int
	mu             sync.Mutex
	refillInterval time.Duration
	quit           chan struct{}
}

// Router is our HTTP router with integrated logging.
type Router struct {
	staticRoutes  *tree.Tree   // static routes stored by exact path
	dynamicRoutes []route      // routes with parameters (e.g., ":id")
	middlewares   []Middleware // middleware chain
	workerPool    *WorkerPool  // optional worker pool for concurrent handling
	rateLimiter   *RateLimiter // optional rate limiter on the critical path
}

type Group struct {
	prefix      string
	middlewares []Middleware
	router      *Router
}

// Param represents a single URL parameter
type Param struct {
	Key   string
	Value string
}

// HandlerFunc defines a function to handle HTTP requests
type HandlerFunc func(*Context) error

// Context represents the context for the current HTTP request
type Context struct {
	Request         *http.Request
	path            string
	method          string
	handlers        []HandlerFunc
	index           int
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
