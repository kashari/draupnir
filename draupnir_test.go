package draupnir

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestRouter returns a router with logging pre-marked as configured so tests
// don't create a default log file.
func newTestRouter() *Router {
	r := New()
	r.loggingConfigured = true
	return r
}

func TestCompilePatternAndMatch(t *testing.T) {
	segs, wildcard := compilePattern("/users/:id/books/:bookID")
	if wildcard {
		t.Fatalf("did not expect wildcard")
	}
	if len(segs) != 4 || segs[1].kind != segParam || segs[1].value != "id" {
		t.Fatalf("unexpected segments: %+v", segs)
	}

	rt := route{segments: segs}
	var params []Param
	parts := splitPath("/users/42/books/7")
	if !rt.matchParts(parts, &params) {
		t.Fatalf("expected match")
	}
	got := map[string]string{}
	for _, p := range params {
		got[p.Key] = p.Value
	}
	if got["id"] != "42" || got["bookID"] != "7" {
		t.Fatalf("unexpected params: %+v", got)
	}

	// Length mismatch must not match.
	params = params[:0]
	if rt.matchParts(splitPath("/users/42/books"), &params) {
		t.Fatalf("expected no match on length mismatch")
	}
}

func TestWildcardMatch(t *testing.T) {
	segs, wildcard := compilePattern("/files/*path")
	if !wildcard {
		t.Fatalf("expected wildcard")
	}
	rt := route{segments: segs, wildcard: true}

	cases := map[string]string{
		"/files/a/b/c.txt": "a/b/c.txt",
		"/files/only":      "only",
		"/files":           "",
	}
	for path, want := range cases {
		var params []Param
		if !rt.matchParts(splitPath(path), &params) {
			t.Fatalf("expected %q to match", path)
		}
		if got := params[len(params)-1].Value; got != want {
			t.Fatalf("path %q: got %q want %q", path, got, want)
		}
	}
}

func TestServeParams(t *testing.T) {
	r := newTestRouter()
	r.GET("/hello/:name", func(c *Context) {
		c.String(200, "hi %s", c.Param("name"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello/world", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != 200 || rec.Body.String() != "hi world" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestStaticMultiMethod(t *testing.T) {
	r := newTestRouter()
	r.GET("/x", func(c *Context) { c.String(200, "get") })
	r.POST("/x", func(c *Context) { c.String(200, "post") })

	for method, want := range map[string]string{http.MethodGet: "get", http.MethodPost: "post"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, "/x", nil))
		if rec.Body.String() != want {
			t.Fatalf("%s: got %q want %q", method, rec.Body.String(), want)
		}
	}

	// Unregistered method -> 405 with Allow header listing both.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/x", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	allow := rec.Header().Get("Allow")
	if !strings.Contains(allow, "GET") || !strings.Contains(allow, "POST") {
		t.Fatalf("unexpected Allow header: %q", allow)
	}
}

func TestMiddlewareOrder(t *testing.T) {
	r := newTestRouter()
	var order []string
	r.Use(func(c *Context) {
		order = append(order, "a-before")
		c.Next()
		order = append(order, "a-after")
	})
	r.Use(func(c *Context) {
		order = append(order, "b-before")
		c.Next()
		order = append(order, "b-after")
	})
	r.GET("/", func(c *Context) {
		order = append(order, "handler")
		c.String(200, "ok")
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"a-before", "b-before", "handler", "b-after", "a-after"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestMiddlewareAbort(t *testing.T) {
	r := newTestRouter()
	reached := false
	r.Use(func(c *Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	r.GET("/", func(c *Context) {
		reached = true
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if reached {
		t.Fatalf("handler ran despite abort")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestContextStoreNoPanic(t *testing.T) {
	r := newTestRouter()
	r.Use(func(c *Context) {
		c.Set("user", "alice") // would panic on a nil map before the fix
		c.Next()
	})
	r.GET("/", func(c *Context) {
		c.String(200, c.GetString("user"))
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Body.String() != "alice" {
		t.Fatalf("got %q", rec.Body.String())
	}
}

func TestJSONEscaping(t *testing.T) {
	r := newTestRouter()
	r.GET("/", func(c *Context) {
		c.JSON(200, map[string]string{"msg": `he said "hi"` + "\n"})
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	// Must be valid, escaped JSON.
	if !strings.Contains(rec.Body.String(), `\"hi\"`) {
		t.Fatalf("quote not escaped: %s", rec.Body.String())
	}
}

func TestConcurrencyLimiterRejects(t *testing.T) {
	r := newTestRouter().WithWorkerPool(1)
	block := make(chan struct{})
	entered := make(chan struct{})
	r.GET("/slow", func(c *Context) {
		close(entered)
		<-block
	})

	go r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
	<-entered // first request now holds the only slot

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when saturated, got %d", rec.Code)
	}
	close(block)
}

func TestBrokerFanout(t *testing.T) {
	b := NewBroker()
	s1 := b.Subscribe("news")
	s2 := b.Subscribe("news")

	b.Publish("news", Event{Data: []byte("hello")})

	for i, sub := range []*Subscription{s1, s2} {
		select {
		case ev := <-sub.C:
			if string(ev.Data) != "hello" {
				t.Fatalf("sub %d got %q", i, ev.Data)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d did not receive event", i)
		}
	}

	// After unsubscribe, s1 stops receiving; s2 still does.
	b.Unsubscribe(s1)
	if _, open := <-s1.C; open {
		t.Fatalf("expected s1 channel closed after unsubscribe")
	}
	b.Publish("news", Event{Data: []byte("again")})
	select {
	case ev := <-s2.C:
		if string(ev.Data) != "again" {
			t.Fatalf("s2 got %q", ev.Data)
		}
	case <-time.After(time.Second):
		t.Fatalf("s2 did not receive second event")
	}
}

func TestBrokerSlowConsumerDrop(t *testing.T) {
	b := NewBroker(WithSubscriberBuffer(1))
	sub := b.Subscribe("t")
	// Fill the buffer then publish more; must not block or panic.
	b.Publish("t", Event{Data: []byte("1")})
	b.Publish("t", Event{Data: []byte("2")}) // dropped
	if len(sub.C) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(sub.C))
	}
}
