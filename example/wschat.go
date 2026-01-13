package example

import (
	"fmt"
	"sync"

	"github.com/kashari/draupnir"
	"github.com/kashari/golog"
)

var clients = make(map[*draupnir.WebSocketConn]string)

var broadcastMu sync.Mutex

func main() {
	golog.Init("./dr.log")
	router := draupnir.New()

	router.WEBSOCKET("/ws/chat", func(w *draupnir.WebSocketConn) {

		name := fmt.Sprintf("%s", w.QueryParam("name"))
		if name == "" {
			return
		}

		broadcastMu.Lock()
		clients[w] = name
		broadcastMu.Unlock()

		defer func() {
			broadcastMu.Lock()
			delete(clients, w)
			broadcastMu.Unlock()

			broadcastMessage("Server", fmt.Sprintf("%s has left the chat", name))
		}()

		for msg := range w.ReceiveChan {
			broadcastMessage(name, string(msg))
		}
	})

	if err := router.Start("1556"); err != nil {
		golog.Error("Error starting server on port 1556: {}", err)
	}
}

func broadcastMessage(senderName string, message string) {
	formattedMsg := fmt.Sprintf("%s: %s", senderName, message)

	broadcastMu.Lock()
	defer broadcastMu.Unlock()

	for client := range clients {
		client.Send([]byte(formattedMsg))
	}
}