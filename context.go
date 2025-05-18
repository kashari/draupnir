package draupnir

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kashari/draupnir/ws"
	"github.com/kashari/golog"
)

func (c *Context) Param(key string) string {
	if params, ok := c.Request.Context().Value(ctxKey("params")).(map[string]string); ok {
		return params[key]
	}
	return ""
}

// ParamInt gets a path parameter as int by key
func (c *Context) ParamInt(key string) (int, error) {
	return strconv.Atoi(c.Param(key))
}

// ParamInt64 gets a path parameter as int64 by key
func (c *Context) ParamInt64(key string) (int64, error) {
	return strconv.ParseInt(c.Param(key), 10, 64)
}

// ParamFloat64 gets a path parameter as float64 by key
func (c *Context) ParamFloat64(key string) (float64, error) {
	return strconv.ParseFloat(c.Param(key), 64)
}

// ParamBool gets a path parameter as bool by key
func (c *Context) ParamBool(key string) (bool, error) {
	return strconv.ParseBool(c.Param(key))
}

// Query gets a query parameter by key
func (c *Context) Query(key string) string {
	if !c.queryParsed {
		c.query = c.Request.URL.Query()
		c.queryParsed = true
	}
	return c.query.Get(key)
}

// QueryInt gets a query parameter as int by key
func (c *Context) QueryInt(key string) (int, error) {
	return strconv.Atoi(c.Query(key))
}

// func (c *Context) WebSocket() (*websocket.Conn, error) {
// 	return upgrader.Upgrade(c.Writer.ResponseWriter, c.Request, nil)
// }

// QueryInt64 gets a query parameter as int64 by key
func (c *Context) QueryInt64(key string) (int64, error) {
	return strconv.ParseInt(c.Query(key), 10, 64)
}

// QueryFloat64 gets a query parameter as float64 by key
func (c *Context) QueryFloat64(key string) (float64, error) {
	return strconv.ParseFloat(c.Query(key), 64)
}

// QueryBool gets a query parameter as bool by key
func (c *Context) QueryBool(key string) (bool, error) {
	return strconv.ParseBool(c.Query(key))
}

// QueryArray gets a query parameter as a string slice by key
func (c *Context) QueryArray(key string) []string {
	if !c.queryParsed {
		c.query = c.Request.URL.Query()
		c.queryParsed = true
	}
	return c.query[key]
}

// GetQueries gets all query parameters
func (c *Context) GetQueries() url.Values {
	if !c.queryParsed {
		c.query = c.Request.URL.Query()
		c.queryParsed = true
	}
	return c.query
}

// FormValue gets a form parameter by key
func (c *Context) FormValue(key string) string {
	if !c.formParsed {
		c.parseForm()
	}
	return c.formValues.Get(key)
}

// FormValueInt gets a form parameter as int by key
func (c *Context) FormValueInt(key string) (int, error) {
	return strconv.Atoi(c.FormValue(key))
}

// FormValueInt64 gets a form parameter as int64 by key
func (c *Context) FormValueInt64(key string) (int64, error) {
	return strconv.ParseInt(c.FormValue(key), 10, 64)
}

// FormValueFloat64 gets a form parameter as float64 by key
func (c *Context) FormValueFloat64(key string) (float64, error) {
	return strconv.ParseFloat(c.FormValue(key), 64)
}

// FormValueBool gets a form parameter as bool by key
func (c *Context) FormValueBool(key string) (bool, error) {
	return strconv.ParseBool(c.FormValue(key))
}

// FormValueArray gets a form parameter as a string slice by key
func (c *Context) FormValueArray(key string) []string {
	if !c.formParsed {
		c.parseForm()
	}
	return c.formValues[key]
}

// GetFormValues gets all form parameters
func (c *Context) GetFormValues() url.Values {
	if !c.formParsed {
		c.parseForm()
	}
	return c.formValues
}

// parseForm parses form data
func (c *Context) parseForm() error {
	if c.formParsed {
		return nil
	}

	contentType := c.Request.Header.Get(HeaderContentType)
	if strings.HasPrefix(contentType, MIMEApplicationForm) {
		if err := c.Request.ParseForm(); err != nil {
			return err
		}
		c.formValues = c.Request.Form
	} else {
		c.formValues = make(url.Values)
	}

	c.formParsed = true
	return nil
}

// FormFile gets a file from multipart form data
func (c *Context) FormFile(key string) (*multipart.FileHeader, error) {
	if !c.multipartParsed {
		if err := c.parseMultipartForm(); err != nil {
			return nil, err
		}
	}
	if c.multipartForm == nil {
		return nil, http.ErrMissingFile
	}
	return c.multipartForm.File[key][0], nil
}

// FormFiles gets all files for a key from multipart form data
func (c *Context) FormFiles(key string) ([]*multipart.FileHeader, error) {
	if !c.multipartParsed {
		if err := c.parseMultipartForm(); err != nil {
			return nil, err
		}
	}
	if c.multipartForm == nil {
		return nil, http.ErrMissingFile
	}
	return c.multipartForm.File[key], nil
}

// parseMultipartForm parses multipart form data
func (c *Context) parseMultipartForm() error {
	if c.multipartParsed {
		return nil
	}

	contentType := c.Request.Header.Get(HeaderContentType)
	if strings.HasPrefix(contentType, MIMEMultipartForm) {
		// 32 MB max memory
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return err
		}
		c.multipartForm = c.Request.MultipartForm
	}

	c.multipartParsed = true
	return nil
}

// Set sets a value in the context store
func (c *Context) Set(key string, value interface{}) {
	c.store[key] = value
}

// Get gets a value from the context store
func (c *Context) Get(key string) (interface{}, bool) {
	val, ok := c.store[key]
	return val, ok
}

// GetString gets a string value from the context store
func (c *Context) GetString(key string) string {
	if val, ok := c.store[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt gets an int value from the context store
func (c *Context) GetInt(key string) int {
	if val, ok := c.store[key]; ok {
		if i, ok := val.(int); ok {
			return i
		}
	}
	return 0
}

// GetInt64 gets an int64 value from the context store
func (c *Context) GetInt64(key string) int64 {
	if val, ok := c.store[key]; ok {
		if i, ok := val.(int64); ok {
			return i
		}
	}
	return 0
}

// GetFloat64 gets a float64 value from the context store
func (c *Context) GetFloat64(key string) float64 {
	if val, ok := c.store[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
	}
	return 0
}

// GetBool gets a bool value from the context store
func (c *Context) GetBool(key string) bool {
	if val, ok := c.store[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// BindJSON binds JSON body to a struct
func (c *Context) BindJSON(obj interface{}) error {
	return c.decodeJSON(obj)
}

// decodeJSON decodes JSON body to a struct
func (c *Context) decodeJSON(obj interface{}) error {
	// Get the content type
	contentType := c.Request.Header.Get(HeaderContentType)
	if !strings.HasPrefix(contentType, MIMEApplicationJSON) {
		return errors.New("content-type is not application/json")
	}

	// Read the body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	defer c.Request.Body.Close()

	// Reset the body
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// Use a faster JSON decoder
	return jsonUnmarshal(body, obj)
}

// JSON sends a JSON response
func (c *Context) JSON(code int, obj interface{}) error {
	c.Writer.Header().Set(HeaderContentType, MIMEApplicationJSON)
	c.Writer.WriteHeader(code)
	c.statusCode = code

	data, err := jsonMarshal(obj)
	if err != nil {
		return err
	}

	_, err = c.Writer.Write(data)
	return err
}

// String sends a string response
func (c *Context) String(code int, format string, values ...any) error {
	c.Writer.Header().Set(HeaderContentType, MIMETextPlain)
	c.Writer.WriteHeader(code)
	c.statusCode = code

	if len(values) > 0 {
		golog.Debug("{} {}", format, values)
		_, err := c.Writer.Write(fmt.Appendf(nil, format, values...))
		return err
	}

	_, err := c.Writer.Write([]byte(format))
	return err
}

// HTML sends an HTML response
func (c *Context) HTML(code int, html string) error {
	c.Writer.Header().Set(HeaderContentType, MIMETextHTML)
	c.Writer.WriteHeader(code)
	c.statusCode = code

	_, err := c.Writer.Write([]byte(html))
	return err
}

// File sends a file response
func (c *Context) File(filepath string) error {
	http.ServeFile(c.Writer, c.Request, filepath)
	return nil
}

// Stream sends a stream response with optional content type
func (c *Context) Stream(contentType string, r io.Reader) error {
	if contentType != "" {
		c.Writer.Header().Set(HeaderContentType, contentType)
	}
	_, err := io.Copy(c.Writer, r)
	return err
}

// StreamFile streams a file efficiently
func (c *Context) StreamFile(file io.ReadSeeker, filename string) error {
	http.ServeContent(c.Writer, c.Request, filename, time.Time{}, file)
	return nil
}

func (c *Context) Streamer(path string) error {
	golog.Debug("Streaming file: {}", path)
	file, err := os.Open(path)
	if err != nil {
		golog.Error("Failed to open file: {}", err)
		return err
	}

	return c.StreamFile(file, path)
}

// Status sets the HTTP status code
func (c *Context) Status(code int) *Context {
	c.statusCode = code
	return c
}

// Header sets a response header
func (c *Context) Header(key, value string) *Context {
	c.Writer.Header().Set(key, value)
	return c
}

// Write writes data to the response
func (c *Context) Write(data []byte) (int, error) {
	return c.Writer.Write(data)
}

// WriteString writes a string to the response
func (c *Context) WriteString(data string) (int, error) {
	return c.Writer.Write([]byte(data))
}

// SetCookie sets a cookie
func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.Writer, cookie)
}

// Cookies returns all cookies
func (c *Context) Cookies() []*http.Cookie {
	return c.Request.Cookies()
}

// Cookie returns a cookie by name
func (c *Context) Cookie(name string) (*http.Cookie, error) {
	return c.Request.Cookie(name)
}

// ClientIP tries to get the client's real IP address
func (c *Context) ClientIP() string {
	if ip := c.Request.Header.Get(HeaderXForwardedFor); ip != "" {
		i := strings.IndexByte(ip, ',')
		if i > 0 {
			return ip[:i]
		}
		return ip
	}

	if ip := c.Request.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	ip, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	return ip
}

// WithContext sets the request context
func (c *Context) WithContext(ctx context.Context) *Context {
	c.Request = c.Request.WithContext(ctx)
	return c
}

// RequestContext returns the request context
func (c *Context) RequestContext() context.Context {
	return c.Request.Context()
}

// URL returns the current URL
func (c *Context) URL() *url.URL {
	return c.Request.URL
}

// Path returns the current path
func (c *Context) Path() string {
	return c.path
}

// Method returns the current HTTP method
func (c *Context) Method() string {
	return c.method
}

// IsWebSocket returns true if the request is a WebSocket upgrade request
func (c *Context) IsWebSocket() bool {
	upgrade := c.Request.Header.Get("Upgrade")
	return strings.ToLower(upgrade) == "websocket"
}

// SwitchToWebSocket upgrades the connection to WebSocket
// and returns the WebSocket connection
//
// Note: This function is not thread-safe and should be called only once
// for each request. It is the caller's responsibility to ensure
// that the connection is not used after it has been closed.
// It is also the caller's responsibility to close the connection
// when it is no longer needed.
// The connection will be closed automatically when the request is done.
func (c *Context) SwitchToWebSocket() (*ws.Conn, error) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return nil, err
	}
	// c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxKey("ws"), conn))
	return conn, nil
}
