package messaging

import (
	"areo/go-chat-backend/content"
	"github.com/go-chi/chi/v5"
	"github.com/gofrs/uuid"
	"github.com/gorilla/websocket"
	"log/slog"
	"net/http"
)

var clients = make(map[*websocket.Conn]bool) // connected clients
var broadcast = make(chan Protocol)          // broadcast channel

var upgrader = websocket.Upgrader{
	// we can whitelist signed in users by looking at their ip address as signin time
	// need to complete auth proto on websocket
	CheckOrigin: func(r *http.Request) bool {
		slog.Debug("checking origin", slog.String("host", r.Host), slog.String("referer", r.Referer()))
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func Configure(router *chi.Mux) {
	router.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleConnections(w, r)
	})

	go handleMessages()
}

type Typing struct {
	Email  string    `json:"email"`
	UserID uuid.UUID `json:"userId"`
	ID     uuid.UUID `json:"id"`
}

type Like struct {
	Email     string    `json:"email"`
	UserID    uuid.UUID `json:"userId"`
	MessageID uuid.UUID `json:"id"`
}

type Read struct {
	Email     string    `json:"email"`
	UserID    uuid.UUID `json:"userId"`
	MessageID uuid.UUID `json:"id"`
}

type Call struct {
	RecipientID string    `json:"recipientID"`
	UserID      uuid.UUID `json:"userId"`
	ID          uuid.UUID `json:"id"`
	Connection  string    `json:"connection"`
}

type Protocol struct {
	Token   string           `json:"token"` // our bearer token
	Type    string           `json:"type"`
	ID      string           `json:"id"`
	Message *content.Message `json:"message"`
	Typ     *Typing          `json:"typing"`
	Call    *Call            `json:"call"`
	Like    *Like            `json:"like"`
	Read    *Read            `json:"read"`
}

func handleConnections(w http.ResponseWriter, r *http.Request) {

	upgrader.CheckOrigin = CheckAuth

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("unable to upgrade request to web socket", slog.Any("err", err))
	}
	defer ws.Close()

	// Register our new client
	// have seen concurrent map writes here, TODO: should synchronize access

	// fatal error: concurrent map writes
	//
	// goroutine 1302 [running]:
	// runtime.throw({0x102fd95bf?, 0x0?})
	// /usr/local/go/src/runtime/panic.go:992 +0x50 fp=0x1400044ea90 sp=0x1400044ea60 pc=0x10276e980
	// runtime.mapassign_fast64ptr(0x1039354a0?, 0x12acb2180?, 0x1400052a420)
	// /usr/local/go/src/runtime/map_fast64.go:192 +0x310 fp=0x1400044ead0 sp=0x1400044ea90 pc=0x10274c640
	// areo/go-chat-backend/messaging.handleConnections({0x12acb2180?, 0x140000ba100?}, 0x14000169290?)
	// /Users/thor/go/src/areo/go-chat-backend/messaging/chatserver.go:61 +0x128 fp=0x1400044f450 sp=0x1400044ead0 pc=0x102d4d598

	clients[ws] = true

	for {
		var proto Protocol
		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&proto)
		if err != nil {
			slog.Error("unable to read message", slog.Any("err", err))
			//delete(clients, ws) // we don't currently disconnect..
			break
		}
		slog.Debug("chat", slog.Any("throb", proto.Typ), slog.Any("payload", proto.Message))

		if proto.Type == "auth" && proto.Message != nil {
			slog.Debug("received auth msg", slog.Any("payload", proto.Message))
			// TODO
		} else if proto.Type == "msg" && proto.Message != nil {
			// persist message
			*proto.Message, err = content.SaveMessage(*proto.Message) // use returned msg to get message id
			if err != nil {
				slog.Error("unable to save message", slog.Any("error", err))
			}
		} else if proto.Type == "typ" {
		} else if proto.Type == "react" {
			content.LikePost(proto.Like.MessageID, proto.Like.UserID)
		} else if proto.Type == "read" {
		} else if proto.Type == "call" {
			slog.Debug("got call", slog.String("userID", proto.Call.UserID.String()),
				slog.String("recipientID", proto.Call.RecipientID), slog.String("conn", proto.Call.Connection))
		} else if proto.Type == "answer" {
		} else if proto.Type == "candidate" {
		}

		broadcast <- proto
	}
}

func handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		proto := <-broadcast

		// Send it out to every client that is currently connected
		for client := range clients {

			// is the client subscribed / permitted this message?
			// TODO

			err := client.WriteJSON(proto)
			if err != nil {
				slog.Error("unable to broadcast message", slog.Any("err", err))
				client.Close()
				delete(clients, client)
			}
		}
	}
}

func CheckAuth(r *http.Request) bool {
	// oauth token present and valid?
	// TODO
	return true
}
