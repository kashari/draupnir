package main

import (
	"time"

	"github.com/kashari/draupnir"
	"github.com/kashari/draupnir/ws"
	"github.com/kashari/golog"
)

func main() {
	router := draupnir.New().WithFileLogging("example.log").
		WithWorkerPool(10).
		WithRateLimiter(10, 1*time.Second)

	router.GET("/", func(ctx *draupnir.Context) {
		ctx.String(200, "Hello, World!")
	})

	router.GET("/hello/:name", func(ctx *draupnir.Context) {
		name := ctx.Param("name")
		ctx.String(200, "Hello, %s!", name)
	})

	router.POST("/submit", func(ctx *draupnir.Context) {
		var data struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		if err := ctx.BindJSON(&data); err != nil {
			ctx.String(400, "Invalid JSON: %s", err)
			return
		}
		ctx.String(200, "Received: Name=%s, Age=%d", data.Name, data.Age)
	})

	router.GET("/stream", func(ctx *draupnir.Context) {
		filepath := ctx.Query("file")
		if filepath == "" {
			ctx.String(400, "File path is required")
			return
		}

		ctx.Streamer(filepath)
	})

	router.WEBSOCKET("/v1/ws/chat", func(ws *draupnir.WebSocketConn) {
		// Send initial message
		ws.Send([]byte("Welcome!"))

		// Process incoming messages
		for msg := range ws.ReceiveChan {
			golog.Info("Received message: {}", string(msg))
			ws.Send([]byte("Response to: " + string(msg)))
		}
	})

	router.GET("/ws/chat", func(ctx *draupnir.Context) {
		conn, err := ctx.SwitchToWebSocket()
		if err != nil {
			ctx.String(500, "Failed to upgrade to WebSocket: %s", err)
			return
		}
		defer conn.Close()
		conn.WriteMessage(ws.TextMessage, []byte("Welcome to the Draupnir WebSocket chat!"))
		golog.Info("WebSocket connection established")
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				golog.Error("Failed to read message: {}", err)
				break
			}
			golog.Info("Received message: {}", string(msg))
			if msgType == ws.TextMessage {
				conn.WriteMessage(ws.TextMessage, []byte("Echo: "+string(msg)))
			} else if msgType == ws.BinaryMessage {
				golog.Info("Received binary message")
			}
		}
		golog.Info("WebSocket connection closed")
	})

	router.GET("/chat", func(ctx *draupnir.Context) {
		ctx.HTML(200, `
			<!DOCTYPE html>
			<html lang="en">
			<head>
				<meta charset="UTF-8">
				<meta name="viewport" content="width=device-width, initial-scale=1.0">
				<title>Draupnir WebSocket Test Client</title>
				<style>
					body {
						font-family: Arial, sans-serif;
						max-width: 800px;
						margin: 0 auto;
						padding: 20px;
					}
					#chat-container {
						border: 1px solid #ccc;
						border-radius: 5px;
						padding: 10px;
						height: 400px;
						overflow-y: auto;
						margin-bottom: 10px;
					}
					#message-form {
						display: flex;
					}
					#message-input {
						flex-grow: 1;
						padding: 8px;
						border: 1px solid #ccc;
						border-radius: 4px;
					}
					#send-button {
						margin-left: 10px;
						padding: 8px 15px;
						background-color: #4CAF50;
						color: white;
						border: none;
						border-radius: 4px;
						cursor: pointer;
					}
					.connection-status {
						padding: 5px 10px;
						margin-bottom: 10px;
						border-radius: 4px;
						display: inline-block;
					}
					.connected {
						background-color: #dff0d8;
						color: #3c763d;
					}
					.disconnected {
						background-color: #f2dede;
						color: #a94442;
					}
					.message {
						margin-bottom: 8px;
						padding: 8px;
						border-radius: 4px;
					}
					.received {
						background-color: #f8f9fa;
						border-left: 3px solid #007bff;
					}
					.sent {
						background-color: #e9f2fd;
						border-left: 3px solid #28a745;
						text-align: right;
					}
				</style>
			</head>
			<body>
				<h1>Draupnir WebSocket Chat</h1>
				
				<div id="status" class="connection-status disconnected">Disconnected</div>
				<button id="connect-button">Connect</button>
				
				<div id="chat-container"></div>
				
				<form id="message-form">
					<input type="text" id="message-input" placeholder="Type your message..." disabled>
					<button type="submit" id="send-button" disabled>Send</button>
				</form>
				
				<script>
					// DOM elements
					const statusEl = document.getElementById('status');
					const connectButton = document.getElementById('connect-button');
					const chatContainer = document.getElementById('chat-container');
					const messageForm = document.getElementById('message-form');
					const messageInput = document.getElementById('message-input');
					const sendButton = document.getElementById('send-button');
					
					// WebSocket connection
					let socket = null;
					
					// Connect to WebSocket server
					connectButton.addEventListener('click', function() {
						if (socket && socket.readyState === WebSocket.OPEN) {
							socket.close();
							return;
						}
						
						// Change protocol (ws/wss) and host as needed for your environment
						const wsUrl = 'ws://' + window.location.host + '/ws/chat';
						socket = new WebSocket(wsUrl);
						
						socket.onopen = function() {
							statusEl.textContent = 'Connected';
							statusEl.className = 'connection-status connected';
							connectButton.textContent = 'Disconnect';
							messageInput.disabled = false;
							sendButton.disabled = false;
							
							addMessage('System', 'Connected to WebSocket server', 'received');
						};
						
						socket.onclose = function() {
							statusEl.textContent = 'Disconnected';
							statusEl.className = 'connection-status disconnected';
							connectButton.textContent = 'Connect';
							messageInput.disabled = true;
							sendButton.disabled = true;
							
							addMessage('System', 'Disconnected from WebSocket server', 'received');
						};
						
						socket.onerror = function(error) {
							console.error('WebSocket error:', error);
							addMessage('System', 'WebSocket error occurred', 'received');
						};
						
						socket.onmessage = function(event) {
							addMessage('Server', event.data, 'received');
						};
					});
					
					// Send message
					messageForm.addEventListener('submit', function(e) {
						e.preventDefault();
						
						const message = messageInput.value.trim();
						if (!message || !socket || socket.readyState !== WebSocket.OPEN) return;
						
						socket.send(message);
						addMessage('You', message, 'sent');
						messageInput.value = '';
					});
					
					// Add message to chat container
					function addMessage(sender, text, type) {
						const messageEl = document.createElement('div');
						messageEl.className = messageEl.className + ' message ' + type;
						messageEl.innerHTML = '<strong>' + sender + ':</strong> ' + text;
						chatContainer.appendChild(messageEl);
						chatContainer.scrollTop = chatContainer.scrollHeight;
					}
				</script>
			</body>
			</html>`)
	})

	err := router.Start("4423")
	if err != nil {
		golog.Error("Failed to start server: {}", err)
	}
}
