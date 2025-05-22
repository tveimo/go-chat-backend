package content

import (
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/users"
	"areo/go-chat-backend/utils"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/gofrs/uuid"
	"github.com/relvacode/iso8601"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"
)

func GetPrivateMessages(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	otherPartyID, err := uuid.FromString(chi.URLParam(r, "id"))

	if err != nil {
		slog.Error("unable to load messages: no otherPartyID provided", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err})
		return
	}

	query := r.URL.Query().Get("query")

	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		slog.Warn("unable to parse count, using default 10", slog.Any("err", err))
		count = 10
	}

	var before time.Time
	beforeISO8601 := r.URL.Query().Get("before")
	if beforeISO8601 != "" {
		// might need this library to support full 8601 date format; https://golangrepo.com/repo/relvacode-iso8601-go-date-time
		before, err = iso8601.ParseString(beforeISO8601)
		//before, err =  time.Parse(time.RFC3339, beforeISO8601)
		if err != nil {
			slog.Error("unable to parse 'before' timestamp", slog.String("beforeISO8601", beforeISO8601), slog.Any("err", err))
		} else {
			slog.Debug("messages before", slog.String("timestamp", before.Format(time.RFC3339)))
		}
	}

	var messages []Message

	stmt := server.DB.Preload("User").Order("created_at desc")
	stmt.Where("(recipient_id = ? and user_id = ?) or (recipient_id = ? and user_id = ?)  and message_type = 'msg'", otherPartyID, user.ID, user.ID, otherPartyID).Limit(count)
	if !before.IsZero() {
		stmt = stmt.Where("created_at < ?", before)
	}

	if query != "" {
		stmt = stmt.Where("title LIKE ? or description LIKE ?", "%"+query+"%", "%"+query+"%")
	}
	err = stmt.Find(&messages).Error

	if err != nil {
		slog.Error("unable to load messages", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}
	// reverse order, most recent message at the bottom
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	render.JSON(w, r, messages)
}

func GetPrivateChats(w http.ResponseWriter, r *http.Request) {

	_ = r.Context().Value("loggedInUser").(users.User)

	otherPartyID := r.URL.Query().Get("id")

	query := r.URL.Query().Get("query")

	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		slog.Warn("unable to parse count, using default 10", slog.Any("err", err))
		count = 10
	}

	var before time.Time
	beforeISO8601 := r.URL.Query().Get("before")
	if beforeISO8601 != "" {
		// might need this library to support full 8601 date format; https://golangrepo.com/repo/relvacode-iso8601-go-date-time
		before, err = iso8601.ParseString(beforeISO8601)
		//before, err =  time.Parse(time.RFC3339, beforeISO8601)
		if err != nil {
			slog.Error("unable to parse 'before' timestamp", slog.String("beforeISO8601", beforeISO8601), slog.Any("err", err))
		} else {
			slog.Debug("messages before", slog.String("timestamp", before.Format(time.RFC3339)))
		}
	}

	var messages []Message

	stmt := server.DB.Preload("User").Order("created_at desc").Where("(recipient_id = ? or user_id = ?) and message_type = 'msg'", otherPartyID, otherPartyID).Limit(count)
	if !before.IsZero() {
		stmt = stmt.Where("created_at < ?", before)
	}

	if query != "" {
		stmt = stmt.Where("title LIKE ? or description LIKE ?", "%"+query+"%", "%"+query+"%")
	}
	err = stmt.Find(&messages).Error

	if err != nil {
		slog.Error("unable to load messages", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}
	// reverse order, most recent message at the bottom
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	render.JSON(w, r, messages)
}

func GetChats2(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	messages := []Message{}

	page := utils.DefaultQuery(r, "page", 0)
	pageSize := utils.DefaultQuery(r, "pageSize", 30)

	var totalCount int64

	var err error
	err = server.DB.Preload("Recipient").Preload("User").Preload("Read").
		Table("messages msg").Select("msg.*").
		Joins("LEFT JOIN messages next on msg.unique_id = next.unique_id and msg.created_at < next.created_at").
		Where("next.created_at is null and msg.message_type = 'msg' and (msg.recipient_id = ? or msg.user_id = ?)", user.ID, user.ID).
		Order("msg.unique_id").
		Find(&messages).Error

	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	} else {
		pageCount := int(math.Ceil(float64(totalCount) / float64(pageSize)))
		render.JSON(w, r, render.M{"items": messages, "totalCount": totalCount, "page": page, "pageSize": pageSize, "offset": page * pageSize, "pageCount": pageCount})
	}
}
