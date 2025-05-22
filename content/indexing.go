package content

import (
	"areo/go-chat-backend/server"
	"github.com/go-chi/render"
	"github.com/gofrs/uuid"
	"log/slog"
	"net/http"
)

func Reindex(w http.ResponseWriter, r *http.Request) {

	var messages []Message
	err := server.DB.Preload("Channel").Preload("User").Preload("Tags").Preload("Likes").Find(&messages).Error
	if err != nil {
		slog.Error("unable to query messages", slog.Any("err", err))
	}
	for _, v := range messages {
		if v.ID != uuid.Nil {
			go IndexMessage(v.ID, v)
		}
	}

	var channels []Channel
	err = server.DB.Preload("Moderators").Preload("Participants").Find(&channels).Error
	if err != nil {
		slog.Error("unable to query channels", slog.Any("err", err))
	}

	for _, v := range channels {
		go IndexChannel(v.ID, v)
	}
}

func Stats(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, server.BleveStats())
}

func IndexMessage(id uuid.UUID, message Message) {
	var item server.ItemDocument

	item.ID = message.ID.String()
	if message.ChannelID != nil {
		item.ChannelID = message.ChannelID.String()
	}
	item.Title = message.Title
	item.Description = message.Message
	item.Type = "message"

	server.IndexEntry(message.ID, item)
}

func IndexChannel(id uuid.UUID, channel Channel) {
	var item server.ItemDocument

	item.ID = channel.ID.String()
	item.ChannelID = channel.ID.String()
	item.Title = channel.Title
	item.Description = channel.Description
	item.Type = "channel"

	server.IndexEntry(channel.ID, item)
}

func Search(w http.ResponseWriter, r *http.Request) {

	freetext := r.URL.Query().Get("query")

	render.JSON(w, r, server.Search(freetext))
}

func SearchMessages(w http.ResponseWriter, r *http.Request) {

	freetext := r.URL.Query().Get("query")

	ids := server.Search(freetext)
	messages := LoadMessages(ids)

	render.JSON(w, r, messages)
}

func SearchChannels(w http.ResponseWriter, r *http.Request) {

	freetext := r.URL.Query().Get("query")

	ids := server.SearchChannels(freetext)
	messages := LoadChannels(ids)

	render.JSON(w, r, messages)
}
