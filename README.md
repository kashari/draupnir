# Draupnir

**Draupnir** is a high-performance, modern HTTP / WebSocket / Server-Sent-Events router library for Go, designed for productivity, flexibility, and speed. It provides a clean, Context-native API, gin-style middleware, built-in rate limiting, a concurrency limiter, WebSocket integration, and a Server-Sent-Events pub/sub bus — all with minimal dependencies.

```go
router := draupnir.New()

router.GET("/hello/:name", func(c *draupnir.Context) {
    c.JSON(200, map[string]string{"hello": c.Param("name")})
})

router.Start("8080")
```

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Routing](#routing)
  - [Path parameters & wildcards](#path-parameters--wildcards)
  - [HTTP method helpers](#http-method-helpers)
  - [Route groups](#route-groups)
- [Middleware](#middleware)
- [The Context](#the-context)
  - [Reading request data](#reading-request-data)
  - [Writing responses](#writing-responses)
  - [Per-request store](#per-request-store)
  - [Request metadata & helpers](#request-metadata--helpers)
  - [Chain control](#chain-control)
- [Concurrency Limiter](#concurrency-limiter)
- [Rate Limiting](#rate-limiting)
- [Logging](#logging)
- [WebSockets](#websockets)
  - [Channel-based API (`WEBSOCKET`)](#channel-based-api-websocket)
  - [Low-level API (`SwitchToWebSocket`)](#low-level-api-switchtowebsocket)
- [Server-Sent Events & the Bus](#server-sent-events--the-bus)
- [API Reference](#api-reference)
- [Performance Notes](#performance-notes)
- [Testing](#testing)
- [Example Project](#example-project)
- [Contributing](#contributing)
- [About the Name](#about-the-name)

---

## Features

- **Fast HTTP routing** — static routes in a radix tree, dynamic routes with precompiled segments.
- **Path parameters and wildcards** — `:name` and trailing `*rest` captures.
- **Context-native, gin-style middleware** — `c.Next()` / `c.Abort()`, per-route, per-group, and global.
- **Route groups** with nested prefixes and inherited middleware.
- **Concurrency limiter** — bound the number of in-flight handlers (sheds load with `503`).
- **Fixed-window rate limiter** — lock-free hot path.
- **WebSockets** — a simple channel-based API and a full-featured low-level API.
- **Server-Sent Events** — an `SSEStream` helper plus a topic-based pub/sub `Broker` for "bus" endpoints.
- **Pooled Contexts** — the request Context is reused to keep per-request allocations low.
- **Integrated logging** via [golog](https://github.com/kashari/golog), auto-initialized when you don't configure it.
- **Minimal dependencies** (`golog` + `golang.org/x/net`).

---

## Installation

```sh
go get github.com/kashari/draupnir
```

Requires Go 1.23+.

---

## Quick Start

```go
package main

import (
    "time"

    "github.com/kashari/draupnir"
    "github.com/kashari/golog"
)

func main() {
    router := draupnir.New().
        WithFileLogging("server.log").
        WithWorkerPool(16).
        WithRateLimiter(100, 1*time.Second)

    router.GET("/", func(c *draupnir.Context) {
        c.String(200, "Welcome to Draupnir!")
    })

    router.GET("/hello/:name", func(c *draupnir.Context) {
        c.JSON(200, map[string]string{"message": "Hello, " + c.Param("name") + "!"})
    })

    router.POST("/api/data", func(c *draupnir.Context) {
        var payload struct {
            Value string `json:"value"`
        }
        if err := c.BindJSON(&payload); err != nil {
            c.String(400, "Invalid JSON: %v", err)
            return
        }
        c.JSON(200, map[string]string{"received": payload.Value})
    })

    if err := router.Start("8080"); err != nil {
        golog.Error("Server error: {}", err)
    }
}
```

`Start(port)` blocks the calling goroutine. The `port` is a string, e.g. `"8080"`.
`New()` returns a `*Router` that is itself an `http.Handler`, so you can also mount it on your own `http.Server` or use it with `httptest`.

---

## Routing

A **handler** is a `draupnir.HandlerFunc`:

```go
type HandlerFunc func(*draupnir.Context)
```

Register routes with the method helpers. Every helper has the signature:

```go
func (r *Router) GET(pattern string, handler HandlerFunc, mw ...HandlerFunc) *Router
```

The optional trailing `mw ...HandlerFunc` are **per-route middleware** (see [Middleware](#middleware)).
All registration methods return the router (or group) for chaining.

### Path parameters & wildcards

| Pattern            | Matches                    | `c.Param(...)`                      |
| ------------------ | -------------------------- | ----------------------------------- |
| `/about`           | `/about` (exact/static)    | —                                   |
| `/users/:id`       | `/users/42`                | `c.Param("id") == "42"`             |
| `/users/:id/posts/:pid` | `/users/42/posts/7`   | `id == "42"`, `pid == "7"`          |
| `/files/*path`     | `/files/a/b/c.txt`         | `c.Param("path") == "a/b/c.txt"`    |

- `:name` captures a **single** path segment.
- `*name` is a **trailing wildcard** that captures the remainder of the path (including slashes). It must be the last segment.
- Static routes (no `:` or `*`) are stored in a radix tree for exact-match lookup; multiple methods can share the same static path (`GET /x` and `POST /x` coexist).

```go
router.GET("/users/:id", func(c *draupnir.Context) {
    id := c.Param("id")
    // typed accessors are available too:
    n, err := c.ParamInt("id")
    _ = n; _ = err
    c.String(200, "user %s", id)
})

router.GET("/assets/*path", func(c *draupnir.Context) {
    c.String(200, "serving %s", c.Param("path"))
})
```

### HTTP method helpers

`GET`, `POST`, `PUT`, `DELETE`, `PATCH`, `OPTIONS`, `HEAD`, `TRACE`, `CONNECT`, and `ANY` are all available on both `*Router` and `*RouterGroup`.

- `HandleFunc(method, pattern, handler, mw...)` — register for an arbitrary method string.
- `Handle(method, pattern, http.HandlerFunc)` — register a classic `net/http` handler.
- `ANY` currently registers the handler under `GET`.
- An `OPTIONS` request matches any registered method for the path.
- A request to a known path with an unregistered method returns `405` with an `Allow` header listing the registered methods.

```go
router.HandleFunc("REPORT", "/docs/:id", reportHandler)     // custom method
router.Handle("GET", "/legacy", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("classic net/http handler"))
})
```

### Route groups

Groups share a path prefix and a middleware stack. Sub-groups inherit the parent's prefix and middleware.

```go
api := router.Group("/api")
api.Use(authMiddleware)          // applies to everything registered on `api` afterwards

api.GET("/profile", profileHandler)

v1 := api.Group("/v1")           // prefix becomes "/api/v1", inherits authMiddleware
v1.GET("/users/:id", getUser)
```

> **Ordering matters.** Middleware are composed into each route's chain **at registration time**, so call `Use(...)` (and register group middleware) **before** the routes they should apply to.

---

## Middleware

Middleware are just `HandlerFunc`s that call `c.Next()` to invoke the rest of the chain. Code before `Next()` runs on the way in; code after runs on the way out.

```go
// Global middleware (applies to routes registered afterwards)
router.Use(func(c *draupnir.Context) {
    start := time.Now()
    c.Next() // run downstream handlers
    golog.Info("{} {} in {}", c.Method(), c.Path(), time.Since(start))
})

// Authentication middleware that can short-circuit the chain
func authMiddleware(c *draupnir.Context) {
    if c.Request.Header.Get("Authorization") == "" {
        c.AbortWithStatus(401) // downstream handlers do NOT run
        return
    }
    c.Set("user", "alice") // stash data for later handlers
    c.Next()
}

router.GET("/admin", adminHandler, authMiddleware) // per-route middleware
```

The composed order for a route is:

```
global middleware → group middleware → per-route middleware → handler
```

**Classic `net/http` middleware** (`func(http.HandlerFunc) http.HandlerFunc`) can be attached with `UseHTTP`:

```go
router.UseHTTP(func(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        golog.Info("Request: {} {}", r.Method, r.URL.Path)
        next(w, r) // call next to continue; omit it to abort the chain
    }
})
```

---

## The Context

`*draupnir.Context` is passed to every handler and middleware. It wraps the request/response and exposes the whole request/response API.

> **Note:** Contexts are pooled and reused between requests. Do not retain a `*Context` (or a value backed by it) after your handler returns. Use `c.RequestContext()` if you need a `context.Context` to hand to a goroutine.

### Reading request data

**Path parameters**

```go
c.Param("id")                 // string ("" if missing)
c.ParamInt("id")              // (int, error)
c.ParamInt64("id")            // (int64, error)
c.ParamFloat64("id")          // (float64, error)
c.ParamBool("flag")           // (bool, error)
```

**Query string** (`?q=go&tag=a&tag=b`)

```go
c.Query("q")                  // "go"
c.QueryInt("page")            // (int, error)
c.QueryInt64("page")          // (int64, error)
c.QueryFloat64("ratio")       // (float64, error)
c.QueryBool("verbose")        // (bool, error)
c.QueryArray("tag")           // ["a", "b"]
c.GetQueries()                // url.Values
```

**Form values** (`application/x-www-form-urlencoded`)

```go
c.FormValue("name")           // string
c.FormValueInt("age")         // (int, error)
c.FormValueInt64("age")       // (int64, error)
c.FormValueFloat64("amount")  // (float64, error)
c.FormValueBool("agree")      // (bool, error)
c.FormValueArray("tags")      // []string
c.GetFormValues()             // url.Values
```

**Multipart file uploads** (`multipart/form-data`, 32 MB in-memory limit)

```go
fileHeader, err := c.FormFile("avatar")   // *multipart.FileHeader
files, err := c.FormFiles("photos")       // []*multipart.FileHeader
```

**JSON body**

```go
var body struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}
if err := c.BindJSON(&body); err != nil { /* 400 */ }
```

`BindJSON` requires a `Content-Type: application/json` header and leaves `c.Request.Body` re-readable.

### Writing responses

```go
c.JSON(200, map[string]any{"ok": true})   // application/json (properly escaped)
c.String(200, "hello %s", name)           // text/plain, printf-style
c.HTML(200, "<h1>Hi</h1>")                // text/html
c.File("/path/to/file.pdf")               // serve a file from disk
c.FileBytes(pdfBytes, "invoice.pdf")      // send bytes as an application/pdf attachment
c.Stream("text/csv", reader)              // stream an io.Reader
c.StreamFile(readSeeker, "data.bin")      // serve an io.ReadSeeker (range-aware)
c.Streamer("/path/to/file")               // open a path and stream it

c.Write(bytes)                            // raw []byte -> (int, error)
c.WriteString("chunk")                    // raw string -> (int, error)
```

**Status, headers, and cookies**

```go
c.Status(201)                             // set status code (chainable)
c.Header("X-Trace", "abc")                // set a response header (chainable)
c.SetCookie(&http.Cookie{Name: "sid", Value: "x"})
c.Cookie("sid")                           // (*http.Cookie, error)
c.Cookies()                               // []*http.Cookie
```

`Status` and `Header` return `*Context`, so they chain: `c.Status(201).Header("X-A", "b")`.

### Per-request store

Share values between middleware and handlers (safe on the pooled Context — the store is reset each request):

```go
c.Set("user", user)
v, ok := c.Get("user")        // (any, bool)
c.GetString("role")           // typed getters with zero-value fallback
c.GetInt("attempts")
c.GetInt64("id")
c.GetFloat64("score")
c.GetBool("admin")
```

### Request metadata & helpers

```go
c.Method()                    // "GET"
c.Path()                      // "/users/42"
c.URL()                       // *url.URL
c.ClientIP()                  // X-Forwarded-For / X-Real-IP / RemoteAddr
c.RequestContext()            // context.Context of the request
c.WithContext(ctx)            // replace the request context (chainable)
c.IsWebSocket()               // true if this is a WebSocket upgrade request
c.Request                     // the underlying *http.Request
c.Writer                      // the underlying http.ResponseWriter
```

### Chain control

```go
c.Next()                      // run the remaining handlers in the chain
c.Abort()                     // stop the chain (remaining handlers won't run)
c.AbortWithStatus(403)        // write a status and abort
c.IsAborted()                 // bool
```

---

## Concurrency Limiter

`WithWorkerPool(n)` caps the number of handlers running **concurrently**. When the limit is reached, additional requests are shed immediately with `503 Service Unavailable`. It adds no goroutines of its own (net/http already serves each request on its own goroutine) — it is a non-blocking semaphore.

```go
router.WithWorkerPool(64) // at most 64 in-flight handlers
```

WebSocket and SSE routes **bypass** the limiter, so long-lived connections never occupy a slot.

---

## Rate Limiting

`WithRateLimiter(maxTokens, refillInterval)` installs a fixed-window limiter: up to `maxTokens` requests are allowed per `refillInterval`, and the bucket refills to full on each interval. The hot path is lock-free. Requests over the limit get `429 Too Many Requests`.

```go
router.WithRateLimiter(100, 1*time.Second) // 100 requests per second
```

You can create a standalone limiter too:

```go
rl := draupnir.NewRateLimiter(100, time.Second)
defer rl.Stop()     // stops the background refill goroutine
if rl.Allow() { /* ... */ }
```

WebSocket and SSE routes bypass the rate limiter.

---

## Logging

Draupnir logs through [golog](https://github.com/kashari/golog). Its format uses `{}` placeholders:

```go
golog.Info("Request: {} {}", method, path)
```

- `WithFileLogging("server.log")` logs to the file **and** the console.
- golog panics if used before it is initialized. If you never call `WithFileLogging` (or `golog.Init` yourself), the router installs a **default file sink** the first time it needs to log — the path is `draupnir.DefaultLogFile` (default `"draupnir.log"`), and you can change it before starting:

```go
draupnir.DefaultLogFile = "logs/app.log"
router := draupnir.New() // no WithFileLogging: default sink is used lazily
```

An external `golog.Init(...)` you call yourself is detected and left untouched.

---

## WebSockets

Draupnir ships **two** independent WebSocket APIs.

### Channel-based API (`WEBSOCKET`)

The simplest option. The handler receives a `*WebSocketConn` with buffered `SendChan` / `ReceiveChan` channels; internal pumps move frames to/from them.

```go
router.WEBSOCKET("/ws/echo", func(ws *draupnir.WebSocketConn) {
    ws.Send([]byte("Welcome!"))
    for msg := range ws.ReceiveChan { // ranges until the client disconnects
        ws.Send([]byte("Echo: " + string(msg)))
    }
})
```

`*WebSocketConn` API:

```go
ws.SendChan      // chan []byte (outgoing)
ws.ReceiveChan   // chan []byte (incoming)
ws.Send(msg)     // enqueue an outgoing message -> error (non-blocking; errors if buffer full/closed)
ws.Close()       // close the connection
ws.QueryParam(k) // read a query-string value from the upgrade request
ws.Header(k)     // read a header from the upgrade request
```

`WEBSOCKET` routes are registered as streaming, so they bypass the rate limiter and concurrency limiter.

### Low-level API (`SwitchToWebSocket`)

For full control (compression, ping/pong, control frames), upgrade inside a normal handler and use the `ws.Conn` from the `github.com/kashari/draupnir/ws` package:

```go
import "github.com/kashari/draupnir/ws"

router.GET("/ws/chat", func(c *draupnir.Context) {
    conn, err := c.SwitchToWebSocket()
    if err != nil {
        c.String(500, "upgrade failed: %v", err)
        return
    }
    defer conn.Close()

    conn.WriteMessage(ws.TextMessage, []byte("hello"))
    for {
        mt, msg, err := conn.ReadMessage()
        if err != nil {
            break
        }
        if mt == ws.TextMessage {
            conn.WriteMessage(ws.TextMessage, append([]byte("echo: "), msg...))
        }
    }
})
```

> **Security:** the default WebSocket `Upgrader` allows **all origins** (`CheckOrigin` returns `true`). Restrict this before exposing WebSockets publicly.

---

## Server-Sent Events & the Bus

Server-Sent Events (SSE) stream events from server to client over a single long-lived HTTP response. Draupnir gives you a low-level stream plus a pub/sub **Broker** for fan-out "bus" endpoints. SSE routes bypass the rate/concurrency limiters.

**Raw SSE stream** — `router.SSE(pattern, handler)`:

```go
router.SSE("/clock", func(c *draupnir.Context, s *draupnir.SSEStream) {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-s.Done():          // client disconnected
            return
        case t := <-ticker.C:
            if s.SendData([]byte(t.Format(time.RFC3339))) != nil {
                return
            }
        }
    }
})
```

**Bus endpoint** — `router.EVENTSTREAM(pattern, topic, broker)` subscribes each client to a topic and streams everything published to it:

```go
bus := draupnir.NewBroker()

// Every client connected to /events receives events published to "news".
router.EVENTSTREAM("/events", "news", bus)

router.POST("/publish", func(c *draupnir.Context) {
    bus.Publish("news", draupnir.Event{
        Event: "news",
        Data:  []byte(c.FormValue("message")),
    })
    c.String(200, "published")
})
```

Test it:

```sh
curl -N localhost:8080/events            # in one terminal (streams)
curl -X POST -d 'message=hello' localhost:8080/publish  # in another
```

**`Event`** — the SSE message (also carried by the Broker):

```go
type Event struct {
    ID    string // SSE "id:"    (optional)
    Event string // SSE "event:" (optional)
    Data  []byte // SSE "data:"  (multi-line payloads are split correctly)
    Retry int    // SSE "retry:" reconnection time in ms (optional)
}
```

**`*SSEStream`** — obtained from `c.SSE()` or handed to your `SSE` handler:

```go
s.Send(event)        // write a full Event and flush -> error
s.SendData(data)     // shorthand for an unnamed data-only event
s.Comment("ping")    // send a ": comment" line (useful as a heartbeat)
s.Done()             // <-chan struct{} closed when the client disconnects
s.Close()            // mark the stream closed
```

**`*Broker`** — topic-based pub/sub. Publishing never blocks: if a subscriber's buffer is full (a slow consumer), the event is dropped for that subscriber.

```go
b := draupnir.NewBroker(draupnir.WithSubscriberBuffer(64)) // per-subscriber buffer (default 16)

sub := b.Subscribe("news")   // *Subscription; read from sub.C (chan Event)
b.Publish("news", ev)        // fan out to all subscribers of the topic
b.Unsubscribe(sub)           // remove a subscriber and close sub.C (idempotent)
b.Topics()                   // []string of topics with subscribers
b.Close()                    // shut down the broker, closing all subscriber channels
```

If you build a custom SSE handler, remember to `Unsubscribe` on disconnect (that's what `EVENTSTREAM` does for you).

---

## API Reference

### Constructors

| Function | Description |
| --- | --- |
| `New() *Router` | Create a router. |
| `NewRateLimiter(maxTokens int, refill time.Duration) *RateLimiter` | Standalone rate limiter. |
| `NewWorkerPool(size int) *WorkerPool` | Standalone concurrency limiter. |
| `NewBroker(opts ...BrokerOption) *Broker` | SSE pub/sub bus. |
| `WithSubscriberBuffer(n int) BrokerOption` | Set per-subscriber buffer size. |

### Router configuration

| Method | Description |
| --- | --- |
| `WithFileLogging(path string) *Router` | Log to a file + console. |
| `WithWorkerPool(size int) *Router` | Cap concurrent handlers. |
| `WithRateLimiter(max int, refill time.Duration) *Router` | Fixed-window rate limiting. |

### Router routing

| Method | Description |
| --- | --- |
| `GET/POST/PUT/DELETE/PATCH/OPTIONS/HEAD/TRACE/CONNECT/ANY(pattern, handler, mw...)` | Register a route. |
| `HandleFunc(method, pattern, handler, mw...)` | Register for an arbitrary method. |
| `Handle(method, pattern, http.HandlerFunc)` | Register a classic handler. |
| `Use(m ...HandlerFunc) *Router` | Add global middleware. |
| `UseHTTP(m Middleware) *Router` | Add classic `net/http` middleware. |
| `Group(prefix string) *RouterGroup` | Create a route group. |
| `WEBSOCKET(pattern, WebSocketHandler) *Router` | Channel-based WebSocket endpoint. |
| `SSE(pattern, func(*Context, *SSEStream), mw...) *Router` | Raw SSE endpoint. |
| `EVENTSTREAM(pattern, topic string, *Broker, mw...) *Router` | Bus endpoint. |
| `ListRoutes() []string` | List registered routes. |
| `ServeHTTP(w, r)` | `http.Handler` implementation. |
| `Start(port string) error` | Start the blocking HTTP server. |

`*RouterGroup` exposes the same routing/method helpers plus `Use` and `Group`.

### Types

| Type | Description |
| --- | --- |
| `HandlerFunc func(*Context)` | Handler and middleware signature. |
| `Middleware func(http.HandlerFunc) http.HandlerFunc` | Classic middleware (for `UseHTTP`). |
| `WebSocketHandler func(*WebSocketConn)` | Channel-based WebSocket handler. |
| `Event` | An SSE event / bus message. |
| `Param` | A single `{Key, Value}` path parameter. |
| `Context`, `Router`, `RouterGroup`, `SSEStream`, `Broker`, `Subscription`, `RateLimiter`, `WorkerPool` | Core types. |

### Variables

| Variable | Description |
| --- | --- |
| `DefaultLogFile string` | Path used for the default log sink when logging isn't configured (default `"draupnir.log"`). |

---

## Performance Notes

- **Pooled Contexts** — the per-request `*Context` is recycled via `sync.Pool`, so hot paths avoid an allocation per request. (Consequently: don't retain a `*Context` past your handler.)
- **Params without `context.WithValue`** — path parameters live on the Context, avoiding boxing and a context-chain walk on each `Param` call.
- **Precompiled routes** — dynamic patterns are split into segments once at registration, not per request; static routes use an exact-match radix tree.
- **Lock-free limiters** — the rate limiter uses atomics; the concurrency limiter is a non-blocking channel semaphore.
- **Streaming bypass** — WebSocket/SSE connections skip the limiters so they never hold a slot for their lifetime.

---

## Testing

Draupnir is a standard `http.Handler`, so test it with `net/http/httptest`:

```go
func TestHello(t *testing.T) {
    r := draupnir.New()
    r.GET("/hello/:name", func(c *draupnir.Context) {
        c.String(200, "hi %s", c.Param("name"))
    })

    rec := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/hello/world", nil)
    r.ServeHTTP(rec, req)

    if rec.Body.String() != "hi world" {
        t.Fatalf("got %q", rec.Body.String())
    }
}
```

Run the suite with `go test ./...`.

---

## Example Project

See [`example/main.go`](example/main.go) for a complete server with HTTP, path parameters, wildcards, middleware, a WebSocket chat, and an SSE bus endpoint.

---

## Contributing

Contributions, issues, and feature requests are welcome!
Feel free to open an [issue](https://github.com/kashari/draupnir/issues) or submit a pull request.

---

## About the Name

**Draupnir** (pronounced [ˈdrɔupnez̠]) is a legendary ring in Norse mythology, symbolizing abundance and reliability — just like this router aims to be for your Go web services.
