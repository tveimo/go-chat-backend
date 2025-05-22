package content

import (
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/users"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/gofrs/uuid"
	"github.com/relvacode/iso8601"
	"gorm.io/gorm"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type MessageTag struct {
	ID        uint      `json:"-" gorm:"primary_key"`
	Tag       string    `json:"tag" gorm:"type:varchar(255);"`
	MessageID uuid.UUID `json:"-" gorm:"type:char(36);"`
}

type MessageLike struct {
	server.Base
	UserID    uuid.UUID  `json:"userId" gorm:"type:char(36);uniqueIndex:like_idx_message_id_user_id"`
	User      users.User `json:"user" gorm:"foreignKey:UserID"`
	LikeAt    time.Time  `json:"likeAt"`
	MessageID uuid.UUID  `json:"messageId" gorm:"type:char(36);uniqueIndex:like_idx_message_id_user_id"`
}

// Custom unmarshaller and marshaller for tags
func (t *MessageTag) UnmarshalJSON(p []byte) error {
	var tmp string
	if err := json.Unmarshal(p, &tmp); err != nil {
		slog.Error("unable to unmarshal tag", slog.Any("err", err))
		return err
	}
	t.Tag = tmp
	return nil
}

func (t *MessageTag) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Tag)
}

type Message struct {
	server.Base
	UniqueID    string        `json:"-" gorm:"type:char(40);index:unique_id_ix"`
	Title       string        `json:"title"`
	Message     string        `json:"message"`
	Email       string        `json:"email"`
	Location    string        `json:"location" gorm:"type:char(255);"`
	Deadline    string        `json:"deadline" gorm:"type:char(99);"`
	RecipientID *uuid.UUID    `json:"recipientId" gorm:"type:char(36);"` // has value if personal message
	Recipient   *users.User   `json:"recipient" gorm:"foreignKey:RecipientID;"`
	MessageType string        `json:"messageType" gorm:"type:char(36);"` // type of message; posting | system | reply
	PostingType string        `json:"postingType" gorm:"type:char(36);"` // type of posting: jobad | others
	SystemFlags string        `json:"systemFlags" gorm:"type:char(36);"` // generic field for system messages
	InReplyToID *uuid.UUID    `json:"inReplyToId" gorm:"type:char(36);"`
	Replies     []Message     `json:"replies" gorm:"foreignKey:InReplyToID;"`
	UserID      uuid.UUID     `json:"userId" gorm:"type:char(36);"`
	User        users.User    `json:"user" gorm:"foreignKey:UserID;"` //" references:UserID"`
	ChannelID   *uuid.UUID    `json:"channelId" gorm:"type:char(36);references:ID"`
	Channel     Channel       `json:"channel" gorm:"foreignKey:ChannelID;"`
	Read        []MessageRead `json:"read" gorm:"foreignKey:MessageID"`
	Tags        []MessageTag  `json:"tags" gorm:"foreignKey:MessageID"`

	Likes       []MessageLike `json:"likes" gorm:"foreignKey:MessageID"`
	ExternalURL string        `json:"externalUrl" gorm:"type:char(255)"`
}

type MessageRead struct {
	server.Base
	MessageID uuid.UUID  `json:"messageId" gorm:"type:char(36);uniqueIndex:idx_message_id_user_id"`
	ReadAt    time.Time  `json:"readAt"`
	User      users.User `json:"user" gorm:"foreignKey:UserID"`
	UserID    uuid.UUID  `json:"userId" gorm:"type:char(36);uniqueIndex:idx_message_id_user_id"`
}

type Attachment struct {
	server.Base
	Filename     string
	ContentType  string
	RepositoryID string
	Buffer       []byte
}

func LoadChannelMessages(channelID uuid.UUID, since *time.Time, max int) (messages []Message, err error) {

	err = server.DB.Debug().
		Preload("User").Preload("Likes").
		Find(&messages, "channel_id = ? ", channelID).
		Limit(max).
		Error
	if err != nil {
		slog.Error("unable to load older messages for channel", slog.String("channelID", channelID.String()), slog.Any("err", err))
		return nil, err
	}
	return
}

func GetChannelMessages(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	if user.ID == uuid.Nil {
		render.Status(r, http.StatusForbidden)
		render.JSON(w, r, render.M{"status": "error", "err": "not logged in"})
	}

	// load additional, older messages on demand
	channelID := chi.URLParam(r, "id")

	beforeISO8601 := r.URL.Query().Get("before")
	if beforeISO8601 != "" {
		// might need this library to support full 8601 date format; https://golangrepo.com/repo/relvacode-iso8601-go-date-time
		before, err := iso8601.ParseString(beforeISO8601)
		//before, err =  time.Parse(time.RFC3339, beforeISO8601)
		if err != nil {
			slog.Error("unable to parse 'before' timestamp, ignoring", slog.String("beforeISO8601", beforeISO8601), slog.Any("err", err))
		} else {
			slog.Debug("messages before", slog.String("timestamp", before.Format(time.RFC3339)))
		}
	}

	var messages []Message

	if err := server.DB.Order("created_at").Where("channel_id = ? and (message_type = 'post' or message_type = 'system')", channelID).
		Preload("User").Preload("Read").Preload("Tags").Preload("Likes").
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return server.DB.Where("channel_id = ? AND message_type = 'reply'", channelID)
		}).
		Find(&messages).Error; err != nil {
		slog.Error("unable to load messages", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error", "err": err})
		return
	}
	// reverse order, most recent message at the bottom
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	render.JSON(w, r, messages)
}

func GetMessages(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	query := r.URL.Query().Get("query")

	count, err := strconv.Atoi(r.URL.Query().Get("count"))
	if err != nil {
		slog.Error("unable to parse count", slog.Any("err", err))
		count = 10
	}

	var before time.Time
	beforeISO8601 := r.URL.Query().Get("before")
	if beforeISO8601 != "" {
		// might need this library to support full 8601 date format; https://golangrepo.com/repo/relvacode-iso8601-go-date-time
		before, err = iso8601.ParseString(beforeISO8601)
		//before, err =  time.Parse(time.RFC3339, beforeISO8601)
		if err != nil {
			slog.Error("unable to parse 'before' timestamp, ignoring", slog.String("beforeISO8601", beforeISO8601), slog.Any("err", err))
		} else {
			slog.Debug("messages before", slog.String("timestamp", before.Format(time.RFC3339)))
		}
	}

	var messages []Message

	stmt := server.DB.Preload("Channel").Preload("User").Preload("Tags").Preload("Likes").Order("created_at desc").
		Where("message_type = 'post' AND channel_id IN (?)",
			server.DB.Table("channel_participants").Select("channel_id").Where("user_id = ? AND created_by_id = ?", user.ID, user.ID)).
		Limit(count)
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
		render.JSON(w, r, render.M{"status": "error", "err": err})
		return
	}
	// reverse order, most recent message at the bottom
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	render.JSON(w, r, messages)
}

func LoadMessages(ids []string) (messages []Message) {

	count := 10
	err := server.DB.Preload("Channel").Preload("User").Preload("Tags").Preload("Likes").Order("created_at desc").
		Where("id IN (?)", ids).Limit(count).Find(&messages).Error

	if err != nil {
		slog.Error("unable to load messages", slog.Any("err", err))
		return nil
	}
	return
}

type Tag struct {
	Tag string
}

func (t *Tag) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Tag)
}

func GetTags(w http.ResponseWriter, r *http.Request) {

	query := r.URL.Query().Get("query")

	var tags, tags2 []Tag

	err := server.DB.Select("distinct(tag)").Table("keywords").
		Where("tag like ?", query+"%").
		Find(&tags).Error

	if err != nil {
		slog.Error("unable to query tags", slog.String("query", query), slog.Any("err", err))
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err})
		return
	}

	err = server.DB.Select("distinct(tag)").Table("message_tags").
		Where("tag like ?", query+"%").
		Find(&tags2).Error

	if err != nil {
		slog.Error("unable to query tags", slog.String("query", query), slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error", "err": err})
		return
	}

	s := append(tags, tags2...)

	render.JSON(w, r, s)
}

func GetMessage(w http.ResponseWriter, r *http.Request) {
	messageID, err := uuid.FromString(chi.URLParam(r, "id"))

	if err != nil {
		slog.Error("unable to load message: no id provided")
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error"})
		return
	}
	var message Message
	err = server.DB.
		Preload("Tags").
		Find(&message, "id = ?", messageID.String()).Error
	if err != nil {
		slog.Error("unable to load message for id: %s; %v", messageID.String(), err)
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err})
		return
	}

	render.JSON(w, r, message)
}

func SaveMessage(msg Message) (ret Message, err error) {
	//var id uuid.UUID
	//var err error
	log.Printf("saving message; %v", msg)

	// assert meta-data; we need a MessageType set
	if msg.MessageType == "" {
		if msg.InReplyToID == nil {
			msg.MessageType = "post"
		} else {
			msg.MessageType = "reply"
		}
	}
	if msg.ID == uuid.Nil {

		// Does a channel exist? We autocreate a channel for new personal messages.
		// We create a unique hash from both recipient_id and user_id and XOR together, if they are not equal.
		if msg.ChannelID == nil /*uuid.Nil*/ && msg.RecipientID != nil {

			h1 := sha1.New()
			h1.Write(msg.RecipientID.Bytes())
			hash1 := h1.Sum(nil)

			h2 := sha1.New()
			h2.Write(msg.UserID.Bytes())
			hash2 := h2.Sum(nil)

			slog.Debug("comparing", slog.String("hash1", hex.EncodeToString(hash1)), slog.String("hash2", hex.EncodeToString(hash2)))
			if bytes.Compare(hash1, hash2) == 0 {
				msg.UniqueID = hex.EncodeToString(hash1)
				slog.Debug("hashes are equals, so using hash directly", slog.String("hash", hex.EncodeToString(hash1)))
			} else {
				result := make([]byte, len(hash1))
				for i := range hash1 {
					result[i] = hash1[i] ^ hash2[i]
				}
				msg.UniqueID = hex.EncodeToString(result)
				slog.Debug("hashes are not equals, so using XOR", slog.String("hash", hex.EncodeToString(result)))
			}

		} else {
			// for channel messages, the UniqueID is the hash of the ChannelID
			h := sha1.New()
			h.Write(msg.ChannelID.Bytes())
			hash := h.Sum(nil)
			msg.UniqueID = hex.EncodeToString(hash)
		}

		err = server.DB.Create(&msg).Error

		if err != nil {
			slog.Error("unable to create new message", slog.Any("err", err))
			return ret, err
		}
		ret = msg
	} else {
		log.Printf("updating existing message: %v", msg)
		var orig Message
		err = server.DB.Preload("Tags").Where("id = ?", msg.ID.String()).First(&orig).Error
		if err != nil {
			slog.Error("unable to update message", slog.Any("err", err))
			return ret, err
		}

		msgMod := orig
		msgMod.MessageType = msg.MessageType
		msgMod.PostingType = msg.PostingType
		msgMod.Message = msg.Message
		msgMod.ExternalURL = msg.ExternalURL
		msgMod.Tags = msg.Tags

		err = server.DB.Unscoped().Where("message_id = ?", msg.ID).Delete(MessageTag{}).Error

		if err != nil {
			slog.Error("unable to update message", slog.Any("err", err))
		}

		err = server.DB.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&msgMod).Error

		if err != nil {
			slog.Error("unable to update message", slog.Any("err", err))
		}

		ret = msgMod
	}

	go IndexMessage(ret.ID, ret)

	return ret, nil
}
