package draupnir

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kashari/golog"
)

// WebSocket frame opcodes
const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

// readFrame reads a single WebSocket frame from the connection
func (wsc *websocketConnection) readFrame() (opcode byte, payload []byte, err error) {
	reader := *wsc.bufReader

	// Read first byte (FIN, RSV1-3, Opcode)
	firstByte := make([]byte, 1)
	if _, err = io.ReadFull(reader, firstByte); err != nil {
		return 0, nil, err
	}

	fin := firstByte[0]&0x80 != 0
	opcode = firstByte[0] & 0x0F

	// Read second byte (Mask, Payload Length)
	secondByte := make([]byte, 1)
	if _, err = io.ReadFull(reader, secondByte); err != nil {
		return 0, nil, err
	}

	masked := secondByte[0]&0x80 != 0
	payloadLen := int64(secondByte[0] & 0x7F)

	// Handle extended payload length
	if payloadLen == 126 {
		extendedLen := make([]byte, 2)
		if _, err = io.ReadFull(reader, extendedLen); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(extendedLen))
	} else if payloadLen == 127 {
		extendedLen := make([]byte, 8)
		if _, err = io.ReadFull(reader, extendedLen); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(extendedLen))
	}

	// Read masking key if masked
	var maskingKey []byte
	if masked {
		maskingKey = make([]byte, 4)
		if _, err = io.ReadFull(reader, maskingKey); err != nil {
			return 0, nil, err
		}
	}

	// Read payload
	payload = make([]byte, payloadLen)
	if _, err = io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}

	// Apply masking if needed
	if masked {
		for i := int64(0); i < payloadLen; i++ {
			payload[i] ^= maskingKey[i%4]
		}
	}

	// Handle fragmented messages (we'll only support complete messages for now)
	if !fin {
		// In a full implementation, you would accumulate the fragments
		// until you get a fragment with FIN=1
		return 0, nil, fmt.Errorf("fragmented messages not supported")
	}

	return opcode, payload, nil
}

// writeFrame writes a WebSocket frame to the connection
func (wsc *websocketConnection) writeFrame(opcode byte, payload []byte) error {
	writer := *wsc.bufWriter

	// First byte: FIN bit set, opcode
	firstByte := 0x80 | (opcode & 0x0F) // FIN bit set, with opcode
	if _, err := writer.Write([]byte{byte(firstByte)}); err != nil {
		return err
	}

	// Second byte: No mask bit, payload length
	length := len(payload)
	var secondByte byte
	var extendedLength []byte

	if length < 126 {
		secondByte = byte(length)
	} else if length <= 65535 {
		secondByte = 126
		extendedLength = make([]byte, 2)
		binary.BigEndian.PutUint16(extendedLength, uint16(length))
	} else {
		secondByte = 127
		extendedLength = make([]byte, 8)
		binary.BigEndian.PutUint64(extendedLength, uint64(length))
	}

	if _, err := writer.Write([]byte{secondByte}); err != nil {
		return err
	}

	if extendedLength != nil {
		if _, err := writer.Write(extendedLength); err != nil {
			return err
		}
	}

	// Write payload
	if _, err := writer.Write(payload); err != nil {
		return err
	}

	return nil
}

// WebSocketHandler defines the interface for handling WebSocket connections
type WebSocketHandler func(*WebSocketConn)

// WebSocketConn represents a WebSocket connection
type WebSocketConn struct {
	conn        *websocketConnection
	SendChan    chan []byte
	ReceiveChan chan []byte
	Closed      bool
	mu          sync.Mutex
}

// websocketConnection is our implementation of a WebSocket connection
type websocketConnection struct {
	netConn    net.Conn
	bufReader  *io.Reader
	bufWriter  *io.Writer
	req        *http.Request
	readChan   chan []byte
	writeChan  chan []byte
	closeChan  chan struct{}
	closeOnce  sync.Once
	isClosed   bool
	mu         sync.Mutex
	pingPeriod time.Duration
}

// Send sends a message to the WebSocket client
func (wsc *WebSocketConn) Send(message []byte) error {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()

	if wsc.Closed {
		return fmt.Errorf("connection closed")
	}

	select {
	case wsc.SendChan <- message:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// Close closes the WebSocket connection
func (wsc *WebSocketConn) Close() {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()

	if !wsc.Closed {
		wsc.Closed = true
		close(wsc.SendChan)
		if wsc.conn != nil {
			wsc.conn.close()
		}
	}
}

// WEBSOCKET adds a WebSocket endpoint to the router
func (r *Router) WEBSOCKET(pattern string, handler WebSocketHandler) *Router {
	return r.HandleFunc("GET", pattern, func(c *Context) {
		// Check if the request is a WebSocket upgrade request
		if !isWebSocketUpgrade(c.Request) {
			http.Error(c.Writer, "Not a WebSocket handshake", http.StatusBadRequest)
			return
		}

		// Create a new WebSocket connection
		wsConn, err := upgradeToWebSocket(c.Writer, c.Request)
		if err != nil {
			http.Error(c.Writer, "Could not upgrade to WebSocket", http.StatusInternalServerError)
			return
		}

		// Create our WebSocketConn wrapper
		conn := &WebSocketConn{
			conn:        wsConn,
			SendChan:    make(chan []byte, 256),
			ReceiveChan: make(chan []byte, 256),
			Closed:      false,
		}

		// Start goroutines to handle reading/writing
		go conn.readPump()
		go conn.writePump()

		// Call the handler
		handler(conn)
	})
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade request
func isWebSocketUpgrade(r *http.Request) bool {
	upgrade := r.Header.Get("Upgrade")
	connection := r.Header.Get("Connection")

	return strings.ToLower(upgrade) == "websocket" &&
		strings.Contains(strings.ToLower(connection), "upgrade")
}

// upgradeToWebSocket upgrades an HTTP connection to a WebSocket connection
func upgradeToWebSocket(w http.ResponseWriter, r *http.Request) (*websocketConnection, error) {
	// Verify it's a valid WebSocket request
	if !isWebSocketUpgrade(r) {
		return nil, fmt.Errorf("not a websocket upgrade request")
	}

	// Create headers for upgrade response
	headers := http.Header{}
	headers.Add("Upgrade", "websocket")
	headers.Add("Connection", "Upgrade")

	// Get the WebSocket key and create the accept key
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, fmt.Errorf("Sec-WebSocket-Key header missing")
	}

	// Generate the accept key using WebSocket protocol
	acceptKey := generateAcceptKey(key)
	headers.Add("Sec-WebSocket-Accept", acceptKey)

	// Get protocol if requested
	protocol := r.Header.Get("Sec-WebSocket-Protocol")
	if protocol != "" {
		protocols := strings.Split(protocol, ",")
		// Choose the first protocol for simplicity
		if len(protocols) > 0 {
			headers.Add("Sec-WebSocket-Protocol", strings.TrimSpace(protocols[0]))
		}
	}

	// Hijack the connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("webserver doesn't support hijacking")
	}

	netConn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	// Write the upgrade response
	response := "HTTP/1.1 101 Switching Protocols\r\n"
	for k, v := range headers {
		if len(v) > 0 {
			response += fmt.Sprintf("%s: %s\r\n", k, v[0])
		}
	}
	response += "\r\n"

	if _, err := bufrw.WriteString(response); err != nil {
		netConn.Close()
		return nil, err
	}

	if err := bufrw.Flush(); err != nil {
		netConn.Close()
		return nil, err
	}

	// Create our WebSocket connection
	reader := io.Reader(bufrw)
	writer := io.Writer(bufrw)

	wsConn := &websocketConnection{
		netConn:    netConn,
		bufReader:  &reader,
		bufWriter:  &writer,
		req:        r,
		readChan:   make(chan []byte, 256),
		writeChan:  make(chan []byte, 256),
		closeChan:  make(chan struct{}),
		pingPeriod: 30 * time.Second,
	}

	return wsConn, nil
}

// generateAcceptKey creates the Sec-WebSocket-Accept response key
// per the WebSocket protocol RFC6455
func generateAcceptKey(key string) string {
	const WebSocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(key + WebSocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// readPump processes incoming WebSocket messages
func (wsc *WebSocketConn) readPump() {
	defer wsc.Close()

	for {
		select {
		case <-wsc.conn.closeChan:
			return
		default:
			opcode, payload, err := wsc.conn.readFrame()
			if err != nil {
				golog.Error("Error reading WebSocket frame: {}", err)
				return
			}

			switch opcode {
			case OpText, OpBinary:
				select {
				case wsc.ReceiveChan <- payload:
					// Successfully delivered message
				default:
					// Channel full, possible slow consumer
					golog.Warn("WebSocket receive buffer full, dropping message")
				}

			case OpPing:
				// Respond with pong
				wsc.conn.writeFrame(OpPong, payload)

			case OpPong:
				// Ignore pongs, they're just keeping the connection alive

			case OpClose:
				// Client requested close
				wsc.Close()
				return

			default:
				golog.Warn("Unsupported WebSocket opcode: {}", opcode)
			}
		}
	}
}

// writePump sends outgoing WebSocket messages
func (wsc *WebSocketConn) writePump() {
	pingTicker := time.NewTicker(wsc.conn.pingPeriod)
	defer func() {
		pingTicker.Stop()
		wsc.Close()
	}()

	for {
		select {
		case message, ok := <-wsc.SendChan:
			if !ok {
				// Channel closed, send close frame
				wsc.conn.writeFrame(OpClose, []byte{})
				return
			}

			// Write message as text frame
			if err := wsc.conn.writeFrame(OpText, message); err != nil {
				golog.Error("Error writing WebSocket message: {}", err)
				return
			}

		case <-pingTicker.C:
			// Send ping
			if err := wsc.conn.writeFrame(OpPing, []byte{}); err != nil {
				golog.Error("Error writing WebSocket ping: {}", err)
				return
			}

		case <-wsc.conn.closeChan:
			return
		}
	}
}

// close implementation for websocketConnection
func (wsc *websocketConnection) close() {
	wsc.mu.Lock()
	defer wsc.mu.Unlock()

	if !wsc.isClosed {
		wsc.closeOnce.Do(func() {
			close(wsc.closeChan)
			wsc.isClosed = true
			wsc.netConn.Close()
		})
	}
}
