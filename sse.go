package draupnir

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"sync"
)

// ErrStreamingUnsupported is returned when the underlying ResponseWriter cannot
// flush (required for Server-Sent Events).
var ErrStreamingUnsupported = errors.New("draupnir: response writer does not support streaming")

// ErrStreamClosed is returned when writing to an SSE stream after the client
// disconnected or the stream was closed.
var ErrStreamClosed = errors.New("draupnir: sse stream closed")

// Event is a single Server-Sent Event (also used as the message type carried by
// the Broker).
type Event struct {
	ID    string // optional event id (SSE "id:")
	Event string // optional event name (SSE "event:")
	Data  []byte // payload (SSE "data:"); multi-line payloads are split correctly
	Retry int    // optional client reconnection time in ms (SSE "retry:")
}

// SSEStream writes Server-Sent Events to a single client. Its methods are safe
// for concurrent use.
type SSEStream struct {
	c       *Context
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	closed  bool
}

// SSE upgrades the current request to a Server-Sent Events stream. It sets the
// appropriate headers, flushes them, and returns a stream the handler can write
// events to until the client disconnects.
func (c *Context) SSE() (*SSEStream, error) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, ErrStreamingUnsupported
	}

	h := c.Writer.Header()
	h.Set(HeaderContentType, MIMETextEventStream)
	h.Set(HeaderCacheControl, "no-cache")
	h.Set(HeaderConnection, "keep-alive")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	c.Writer.WriteHeader(http.StatusOK)
	c.statusCode = http.StatusOK
	flusher.Flush()

	return &SSEStream{c: c, w: c.Writer, flusher: flusher}, nil
}

// Send writes an event to the stream and flushes it.
func (s *SSEStream) Send(ev Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStreamClosed
	}

	var b bytes.Buffer
	if ev.ID != "" {
		b.WriteString("id: ")
		b.WriteString(ev.ID)
		b.WriteByte('\n')
	}
	if ev.Event != "" {
		b.WriteString("event: ")
		b.WriteString(ev.Event)
		b.WriteByte('\n')
	}
	if ev.Retry > 0 {
		b.WriteString("retry: ")
		b.WriteString(strconv.Itoa(ev.Retry))
		b.WriteByte('\n')
	}
	// Each newline in the payload starts a new "data:" line per the SSE spec.
	for _, line := range bytes.Split(ev.Data, []byte("\n")) {
		b.WriteString("data: ")
		b.Write(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n') // blank line terminates the event

	if _, err := s.w.Write(b.Bytes()); err != nil {
		s.closed = true
		return err
	}
	s.flusher.Flush()
	return nil
}

// SendData is a shorthand for sending an unnamed data-only event.
func (s *SSEStream) SendData(data []byte) error {
	return s.Send(Event{Data: data})
}

// Comment sends an SSE comment line (": ..."), useful as a keep-alive heartbeat.
func (s *SSEStream) Comment(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStreamClosed
	}
	if _, err := s.w.Write([]byte(": " + text + "\n\n")); err != nil {
		s.closed = true
		return err
	}
	s.flusher.Flush()
	return nil
}

// Done returns a channel closed when the client disconnects.
func (s *SSEStream) Done() <-chan struct{} {
	return s.c.Request.Context().Done()
}

// Close marks the stream closed. It does not close the underlying connection;
// returning from the handler does that.
func (s *SSEStream) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

// SSE registers a Server-Sent Events endpoint. The route is streaming, so it
// bypasses the rate limiter and concurrency limiter. The handler runs until it
// returns (typically when the client disconnects).
func (r *Router) SSE(pattern string, handler func(*Context, *SSEStream), mw ...HandlerFunc) *Router {
	return r.handleStreaming(GET, pattern, func(c *Context) {
		stream, err := c.SSE()
		if err != nil {
			http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			return
		}
		handler(c, stream)
	}, mw...)
}

// EVENTSTREAM registers a turnkey "bus" endpoint: each connecting client is
// subscribed to topic on broker, and every event published to that topic is
// streamed to the client until it disconnects.
func (r *Router) EVENTSTREAM(pattern, topic string, broker *Broker, mw ...HandlerFunc) *Router {
	return r.SSE(pattern, func(c *Context, s *SSEStream) {
		sub := broker.Subscribe(topic)
		defer broker.Unsubscribe(sub)

		done := c.Request.Context().Done()
		for {
			select {
			case <-done:
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				if err := s.Send(ev); err != nil {
					return
				}
			}
		}
	}, mw...)
}
