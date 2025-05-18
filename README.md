# Draupnir

**Draupnir** is a high-performance, modern HTTP router and WebSocket framework for Go, designed for productivity, flexibility, and speed. It provides a clean API, robust middleware support, built-in rate limiting, worker pools, and seamless WebSocket integration—all with minimal dependencies.

---

## ✨ Features

- **Lightning-fast HTTP routing** (static & dynamic routes)
- **Middleware chaining** for request/response processing
- **Worker pool** for concurrent request handling
- **Token bucket rate limiter** for traffic control
- **Integrated logging** with [golog](https://github.com/kashari/golog)
- **WebSocket support** with easy upgrade and message channels
- **Convenient context utilities** for query, form, and path parameters
- **Simple, expressive API** inspired by popular Go frameworks
- **Graceful shutdown** and robust error handling
- **Zero external dependencies** (except for logging and optional WebSocket helpers)
- **MIT/Apache-2.0 compatible license** (GPLv3)

---

## 🚀 Getting Started

### Installation

```sh
go get github.com/kashari/draupnir
```

### Minimal Example

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
        WithWorkerPool(8).
        WithRateLimiter(100, 1*time.Second)

    router.GET("/", func(ctx *draupnir.Context) {
        ctx.String(200, "Welcome to Draupnir!")
    })

    router.GET("/hello/:name", func(ctx *draupnir.Context) {
        name := ctx.Param("name")
        ctx.JSON(200, map[string]string{"message": "Hello, " + name + "!"})
    })

    router.POST("/api/data", func(ctx *draupnir.Context) {
        var payload struct {
            Value string ` + "`json:\"value\"`" + `
        }
        if err := ctx.BindJSON(&payload); err != nil {
            ctx.String(400, "Invalid JSON: %v", err)
            return
        }
        ctx.JSON(200, map[string]string{"received": payload.Value})
    })

    // WebSocket echo endpoint
    router.WEBSOCKET("/ws/echo", func(ws *draupnir.WebSocketConn) {
        for msg := range ws.ReceiveChan {
            ws.Send(msg)
        }
    })

    if err := router.Start("8080"); err != nil {
        golog.Error("Server error: {}", err)
    }
}
```

---

## 🛣️ HTTP Routing

- **Static routes:**  
  `router.GET("/about", handler)`
- **Dynamic routes:**  
  `router.GET("/users/:id", handler)`
- **Wildcard routes:**  
  `router.GET("/files/*filepath", handler)`
- **Method helpers:**  
  `GET`, `POST`, `PUT`, `DELETE`, `PATCH`, `OPTIONS`, `HEAD`, `TRACE`, `CONNECT`, `ANY`
- **Middleware:**  
  `router.Use(loggingMiddleware)`

### Grouping Routes

```go
api := router.Group("/api")
api.Use(authMiddleware)
api.GET("/profile", profileHandler)
```

---

## 🧰 Context Utilities

- `ctx.Param("key")` — Path parameter (e.g., `/users/:id`)
- `ctx.Query("q")` — Query parameter (`?q=search`)
- `ctx.FormValue("field")` — Form value (POST/PUT)
- `ctx.BindJSON(&obj)` — Parse JSON body into struct
- `ctx.JSON(code, obj)` — Send JSON response
- `ctx.String(code, format, args...)` — Send plain text response
- `ctx.File(path)` — Send file as response
- `ctx.Stream(contentType, reader)` — Stream response body
- `ctx.Status(code)` — Set HTTP status code
- `ctx.Header(key, value)` — Set response header
- `ctx.SetCookie(cookie)` — Set cookie

---

## 🔌 Middleware

Add middleware globally or per group:

```go
router.Use(loggingMiddleware)

func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        golog.Info("Request: {} {}", r.Method, r.URL.Path)
        next(w, r)
    }
}
```

---

## ⚡ Worker Pool

Enable concurrent request processing:

```go
router.WithWorkerPool(16) // 16 workers
```

---

## 🚦 Rate Limiting

Configure a token bucket rate limiter:

```go
router.WithRateLimiter(100, 1*time.Second) // 100 requests per second
```

---

## 📡 WebSocket Support

- Upgrade any route to WebSocket with `router.WEBSOCKET(path, handler)`
- Send and receive messages via `SendChan` and `ReceiveChan`
- Example:

```go
router.WEBSOCKET("/ws/chat", func(ws *draupnir.WebSocketConn) {
    ws.Send([]byte("Welcome!"))
    for msg := range ws.ReceiveChan {
        ws.Send([]byte("Echo: " + string(msg)))
    }
})
```

---

## 📋 Logging

- File and console logging via [golog](https://github.com/kashari/golog)
- Enable file logging:  
  `router.WithFileLogging("server.log")`

---

## 🧪 Testing

Draupnir is designed for testability. You can use Go's standard `net/http/httptest` package to test your handlers and middleware.

---

## 📚 Example Project

See [`example/main.go`](example/main.go) for a complete example with HTTP and WebSocket endpoints.

---

## 📖 API Reference

- [Context Methods](#-context-utilities)
- [Router Methods](#-http-routing)
- [WebSocket API](#-websocket-support)
- [Worker Pool](#-worker-pool)
- [Rate Limiter](#-rate-limiting)

---

## 🛡️ License

This project is licensed under the [GNU General Public License v3.0](LICENSE).

---

## 🤝 Contributing

Contributions, issues, and feature requests are welcome!  
Feel free to open an [issue](https://github.com/kashari/draupnir/issues) or submit a pull request.

---

## 🧙 About the Name

**Draupnir** (pronounced [ˈdrɔupnez̠]) is a legendary ring in Norse mythology, symbolizing abundance and reliability—just like this router aims to be for your Go web services.

---

**Made with ❤️ by [@kashari](https://github.com/kashari)**