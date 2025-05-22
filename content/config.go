package content

import (
	"areo/go-chat-backend/server"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

func abort(w http.ResponseWriter, r *http.Request, err error) {
	render.Status(r, http.StatusBadRequest)
	render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
}

func InitSchema() {
	slog.Info("setting up channel / read schema")
	err := server.DB.AutoMigrate(&Read{}, &Channel{})
	if err != nil {
		slog.Error("unable to setup channel / read schema", slog.Any("err", err))
	}
	err = server.DB.AutoMigrate(&MessageRead{}, &MessageTag{}, &MessageLike{}, &Message{})
	if err != nil {
		slog.Error("unable to setup message / tag / like / read schema", slog.Any("err", err))
	}

	slog.Info("setting up participants join table")
	err = server.DB.SetupJoinTable(&Channel{}, "Participants", &ChannelParticipant{})
	if err != nil {
		slog.Error("unable to setup join table for channel participants", slog.Any("err", err))
	}
	server.DB.AutoMigrate(&ChannelParticipant{})

}
