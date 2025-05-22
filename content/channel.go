package content

import (
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/users"
	"areo/go-chat-backend/utils"
	"errors"
	"github.com/anuragkumar19/binding"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"gorm.io/gorm/clause"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type Channel struct {
	server.Base
	Title        string       `json:"title,omitempty" gorm:"type:varchar(255);"`
	Description  string       `json:"description,omitempty" gorm:"type:varchar(255);"`
	Open         bool         `json:"open"`
	Promote      bool         `json:"promote"`
	Participants []users.User `json:"participants" gorm:"many2many:channel_participants"`
	Moderators   []users.User `json:"-" gorm:"many2many:channel_moderators"`
	Messages     []Message    `json:"messages" gorm:"foreignKey:ChannelID"`
	Read         []Read       `json:"read" gorm:"foreignKey:ChannelID"`
	UnreadCount  int          `json:"unread" gorm:"-"`
	Url          string       `json:"url"`
}

type Member struct {
	ID        uuid.UUID `gorm:"PrimaryKey"`
	ChannelID uuid.UUID `gorm:"foreignKey:ID"`
	Since     time.Time
}

type Read struct {
	server.Base
	ChannelID uuid.UUID  `json:"channelId" gorm:"type:char(36);uniqueIndex:idx_channel_id_user_id"`
	ReadAt    time.Time  `json:"readAt"`
	User      users.User `json:"user" gorm:"foreignKey:UserID"`
	UserID    uuid.UUID  `json:"userId" gorm:"type:char(36);uniqueIndex:idx_channel_id_user_id"`
}

type ChannelParticipant struct {
	server.Base
	CreatedByID uuid.UUID `json:"createdById" gorm:"type:char(36)"`
	ChannelID   uuid.UUID `json:"channelId" gorm:"primaryKey"`
	UserID      uuid.UUID `json:"userId" gorm:"primaryKey"`
	Approved    bool      `json:"approved"`
}

func GetChannel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		abort(w, r, err)
		return
	}

	var channel Channel
	err = server.DB.
		Preload("Read").
		Preload("Participants").
		Where("id = ?", id.String()).First(&channel).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	render.JSON(w, r, channel)
}

type ChannelAdd struct {
	Title       string `json:"title,omitempty" gorm:"type:varchar(255);"`
	Description string `json:"description,omitempty" gorm:"type:varchar(255);"`
	Open        bool
	Promote     bool
}

func SubscribeToChannel(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		abort(w, r, err)
		return
	}

	user := r.Context().Value("loggedInUser").(users.User)

	type param struct {
		Subscribe bool      `json:"subscribe" binding:""`
		UserID    uuid.UUID `json:"userId" binding:""`
	}

	subscribeParam := param{}

	// allow sending in read state to set a specific readAt timestamp
	if err := binding.Bind(r, &subscribeParam); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	if subscribeParam.UserID == uuid.Nil {
		subscribeParam.UserID = user.ID
	}

	var channel Channel
	err = server.DB.
		Preload("Read").
		Where("id = ?", id.String()).First(&channel).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	usermod := &users.User{}
	usermod.ID = subscribeParam.UserID

	if subscribeParam.Subscribe {
		var subscriber ChannelParticipant
		subscriber.UserID = usermod.ID
		subscriber.ChannelID = id
		subscriber.Approved = true
		subscriber.CreatedByID = user.ID

		err = server.DB.Where(
			subscriber).
			Assign(subscriber).
			FirstOrCreate(&ChannelParticipant{}).Error

		err = server.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_id"}, {Name: "user_id"}},
			UpdateAll: true,
		}).Create(&subscriber).Error

		UserSubscribed(user, channel)
	} else {
		err = server.DB.Unscoped().Table("channel_participants").
			Where("channel_id = ? and user_id = ?", id, usermod.ID).Delete(&ChannelParticipant{}).Error
	}

	if err != nil {
		slog.Error("unable to subscribe to channel", slog.String("channel", channel.Title), slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	render.JSON(w, r, channel)
}

func UserSubscribed(user users.User, channel Channel) {

	var msg Message
	msg.UserID = user.ID
	msg.MessageType = "system"
	msg.SystemFlags = "subscribed"
	msg.ChannelID = &channel.ID

	err := server.DB.Create(&msg).Error

	if err != nil {
		slog.Error("unable to create new system message", slog.Any("err", err))
	}
}

func GetSubscriptions(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	var pending bool
	if r.URL.Query().Get("pending") == "true" {
		pending = true
	}

	var Subscriptions []ChannelParticipant

	var err error
	if pending {
		err = server.DB.Table("channel_participants").Where("user_id = ? AND (created_by_id != ? OR created_by_id IS NULL)", user.ID, user.ID).Find(&Subscriptions).Error
	} else {
		err = server.DB.Table("channel_participants").Where("user_id = ? AND created_by_id = ?", user.ID, user.ID).Find(&Subscriptions).Error
	}

	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var ids = make([]uuid.UUID, len(Subscriptions))
	for i, v := range Subscriptions {
		ids[i] = v.ChannelID
	}
	render.JSON(w, r, render.M{"data": ids})
}

func GetSuggestions(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	var suggestions []Channel

	err := server.DB.
		Model(&suggestions).Preload("Read").Preload("Participants").Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at desc limit 3")
	}).Where("id NOT IN (?)", server.DB.Table("channel_participants").
		Select("channel_id").
		Where("user_id = ?", user.ID),
	).Model(&suggestions).Order("created_at desc").Find(&suggestions).Error

	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	render.JSON(w, r, render.M{"items": suggestions})
}

func SaveChannel(w http.ResponseWriter, r *http.Request) {

	var id uuid.UUID
	var err error

	user := r.Context().Value("loggedInUser").(users.User)

	idString := chi.URLParam(r, "id")

	if idString == "new" {
		id = uuid.Nil
	} else {
		id, err = uuid.FromString(idString)
		if err != nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
			return
		}
	}

	var channelMod Channel
	if err := binding.Bind(r, &channelMod); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	if len(strings.Trim(channelMod.Title, "")) == 0 {
		slog.Error("empty channel title")
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	// disallow certain fields
	channelMod.ID = uuid.Nil

	var status int
	var channel Channel
	if id == uuid.Nil {
		channel = channelMod

		channel.Moderators = []users.User{user}

		err = server.DB.Create(&channel).Error

		var subscriber ChannelParticipant
		subscriber.UserID = user.ID
		subscriber.ChannelID = channel.ID
		subscriber.Approved = true
		subscriber.CreatedByID = user.ID

		err = server.DB.Where(subscriber).Assign(subscriber).FirstOrCreate(&ChannelParticipant{}).Error

		err = server.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_id"}, {Name: "user_id"}},
			UpdateAll: true,
		}).Create(&subscriber).Error

		status = http.StatusCreated

	} else {
		err = server.DB.Where("id = ?", id).First(&channel).Error
		if err != nil {
			slog.Error("unable to query for channel", slog.Any("err", err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
			return
		}
		err = server.DB.Model(&channel).Updates(channelMod).Error
		status = http.StatusOK
	}

	if err != nil {
		slog.Error("unable to save channel", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	if err != nil {
		slog.Error("unable to save channel", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	go IndexChannel(channel.ID, channel)
	render.Status(r, status)
	render.JSON(w, r, channel)
}

func GetChannels(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	channels := []Channel{}
	query := r.URL.Query().Get("query")
	messages := r.URL.Query().Get("messages") == "true"
	pending := r.URL.Query().Get("pending") == "true"
	all := r.URL.Query().Get("all") == "true"

	page := utils.DefaultQuery(r, "page", 0)
	pageSize := utils.DefaultQuery(r, "pageSize", 30)

	// need to limit preload using sorting, see
	// https://stackoverflow.com/questions/52270257/how-do-i-stop-gorm-from-sorting-my-preload-by-id
	// https://gorm.io/docs/preload.html

	var totalCount int64
	var err error
	if query != "" {
		// TODO for all=false
		err = server.DB.
			Model(&channels).Preload("Read").Preload("Participants").Preload("Messages", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc limit 3")
		}).Where("title LIKE ? or description LIKE ?", "%"+query+"%", "%"+query+"%").Count(&totalCount).Limit(pageSize).Offset(page * pageSize).Order("created_at desc").Find(&channels).Error
	} else {
		if all {
			err = server.DB.
				Model(&channels).Preload("Read").Preload("Participants").Preload("Messages", func(db *gorm.DB) *gorm.DB {
				return db.Order("created_at desc limit 3")
			}).Model(&channels).Count(&totalCount).Limit(pageSize).Offset(page * pageSize).
				Order("created_at desc").Find(&channels).Error
		} else {

			var channelSelect *gorm.DB
			if pending {
				channelSelect = server.DB.Table("channel_participants").Select("channel_id").Where("user_id = ? AND (created_by_id != ? OR created_by_id IS NULL)", user.ID, user.ID)
			} else {
				channelSelect = server.DB.Table("channel_participants").Select("channel_id").Where("user_id = ? AND created_by_id = ?", user.ID, user.ID)
			}

			// See https://stackoverflow.com/questions/63475885/how-to-query-a-many2many-relationship-with-a-where-clause-on-the-association-wit
			err = server.DB.Model(&channels).Preload("Read").Preload("Participants").Preload("Messages", func(db *gorm.DB) *gorm.DB {
				return db.Order("created_at desc limit 3")
			}).Where("id IN (?)", channelSelect).Model(&channels).Count(&totalCount).Limit(pageSize).Offset(page * pageSize).
				Order("created_at desc").Find(&channels).Error
		}
	}
	if messages {
		for i, _ := range channels {
			channels[i].Messages, err = LoadChannelMessages(channels[i].ID, nil, 10)
			channels[i].UnreadCount = len(channels[i].Messages)
			if err != nil {
				slog.Error("unable to load channel messages", slog.String("channelID", channels[i].ID.String()), slog.Any("err", err))
			}
		}
	}

	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	} else {
		for i, _ := range channels {
			unreadCount := len(channels[i].Messages)
			readAt := time.UnixMilli(0)
			for _, r := range channels[i].Read {
				if r.UserID == user.ID {
					readAt = r.ReadAt
					unreadCount = -1
				}
			}

			// loop through messages to get unreadCount
			if unreadCount == -1 {
				unreadCount = 0
				for _, m := range channels[i].Messages {
					if m.CreatedAt.After(readAt) {
						// found one message after
						unreadCount++
					} else {
						// found one message before
					}
				}
			}
			channels[i].UnreadCount = unreadCount
		}

		pageCount := int(math.Ceil(float64(totalCount) / float64(pageSize)))
		render.JSON(w, r, render.M{"items": channels, "totalCount": totalCount, "page": page, "pageSize": pageSize, "offset": page * pageSize, "pageCount": pageCount})
	}
}

func LoadChannels(ids []string) (channels []Channel) {

	count := 10
	err := server.DB.Preload("Messages").Preload("Participants").Preload("Moderators").Order("created_at desc").
		Where("id IN (?)", ids).Limit(count).Find(&channels).Error

	if err != nil {
		slog.Error("unable to load messages", slog.Any("err", err))
		return nil
	}
	return
}

func DeleteChannel(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		abort(w, r, err)
		return
	}

	var channel Channel
	err = server.DB.
		Preload("Read").Where("id = ?", id.String()).Delete(&channel).Error
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	render.JSON(w, r, render.M{"status": "OK"})
}

func LoadChannel(id string) (channel Channel, err error) {

	server.DB.
		Preload("Read").Where("id = ?", id).First(&channel)

	if channel.ID.String() == "" {
		slog.Error("channel not found", slog.String("id", id), slog.Any("err", err))
		return channel, errors.New("channel not found, check id")
	}

	return
}

func ReadChannel(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		render.Status(r, http.StatusPreconditionFailed)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var readMod Read

	readMod.ReadAt = time.Now()
	readMod.ChannelID = id
	readMod.UserID = user.ID

	// We do an upsert so that a read timestamp is inserted or updated if already exists
	// https://stackoverflow.com/questions/48915482/upsert-if-on-conflict-occurs-on-multiple-columns-in-postgres-db
	// https://gorm.io/docs/create.html
	server.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"read_at"}),
	}).Create(&readMod)

	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	render.JSON(w, r, render.M{"status": "OK"})
}

// find unread thread count?
// https://dba.stackexchange.com/questions/69074/mysql-querying-for-latest-messages-in-flat-reply-system
// Do we also mark all replies to the provided posting id?
func ReadPosting(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		render.Status(r, http.StatusPreconditionFailed)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var readMod MessageRead

	// allow sending in read state to set a specific readAt timestamp
	/*if err := render.Bind(r, &readMod); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}*/
	readMod.ReadAt = time.Now()
	readMod.MessageID = id
	readMod.UserID = user.ID

	server.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"read_at"}),
	}).Create(&readMod)

	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	render.JSON(w, r, render.M{"status": "OK"})
}

func LikePosting(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		render.Status(r, http.StatusPreconditionFailed)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var likeMod MessageLike

	// allow sending in read state to set a specific likeAt timestamp
	likeMod.LikeAt = time.Now()
	likeMod.MessageID = id
	likeMod.UserID = user.ID

	server.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"like_at"}),
	}).Create(&likeMod)

	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	render.JSON(w, r, render.M{"status": "OK"})
}

func LikePost(messageID uuid.UUID, userID uuid.UUID) {

	var likeMod MessageLike

	// allow sending in read state to set a specific likeAt timestamp
	likeMod.LikeAt = time.Now()
	likeMod.MessageID = messageID
	likeMod.UserID = userID

	err := server.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"like_at"}),
	}).Create(&likeMod)

	if err != nil {
		slog.Error("unable to store reaction", slog.String("error", err.Error.Error()))
		return
	}
}

func ReadPost(messageID uuid.UUID, userID uuid.UUID) {

	var readMod MessageRead

	// allow sending in read state to set a specific readAt timestamp
	/*if err := render.Bind(r, &readMod); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}*/
	readMod.ReadAt = time.Now()
	readMod.MessageID = messageID
	readMod.UserID = userID

	err := server.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"read_at"}),
	}).Create(&readMod)

	if err != nil {
		slog.Error("unable to persist read message", slog.String("error", err.Error.Error()))
		return
	}
}
