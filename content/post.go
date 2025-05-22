package content

import (
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/utils"
	"errors"
	"github.com/anuragkumar19/binding"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/gofrs/uuid"
	"log/slog"
	"math"
	"net/http"
)

type Post struct {
	server.Base
	Title       string `json:"title,omitempty" gorm:"type:varchar(255);"`
	Description string `json:"description,omitempty" gorm:"type:varchar(255);"`
	Body        string `json:"body" gorm:"type:varchar(4096);"`
	Imageref    string `json:"imageref,omitempty" gorm:"type:char(36);"`
}

func GetPost(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		abort(w, r, err)
		return
	}

	var post Post
	err = server.DB.Where("id = ?", id.String()).First(&post).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	render.JSON(w, r, post)
}

type PostMod interface{}

func SavePost(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	//var userMod UserInterface
	var postMod Post
	if err := binding.Bind(r, &postMod); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	// disallow certain user fields
	postMod.ID = uuid.Nil

	var post Post
	if id == uuid.Nil {
		err = server.DB.Create(&postMod).Error
	} else {
		err = server.DB.Where("id = ?", id).First(&post).Error
		if err != nil {
			slog.Error("unable query for post", slog.Any("err", err))
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
			return
		}
		err = server.DB.Model(&post).Updates(postMod).Error
	}

	if err != nil {
		slog.Error("unable to save post", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	render.JSON(w, r, post)
}

func LatestPosts(w http.ResponseWriter, r *http.Request) {

	posts := []Post{}
	err := server.DB.Where("created_at is not null").Limit(50).Order("created_at desc").Find(&posts).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	} else {
		render.JSON(w, r, posts)
	}
	return
}

func GetPosts(w http.ResponseWriter, r *http.Request) {

	posts := []Post{}
	query := r.URL.Query().Get("query")

	page := utils.DefaultQuery(r, "page", 0)
	pageSize := utils.DefaultQuery(r, "pageSize", 30)

	var totalCount int64
	var err error
	if query != "" {
		err = server.DB.Model(&posts).Where("title LIKE ? or description LIKE ?", "%"+query+"%", "%"+query+"%").Count(&totalCount).Limit(pageSize).Offset(page * pageSize).Order("created_at desc").Find(&posts).Error
	} else {
		err = server.DB.Model(&posts).Where("created_at is not null").Count(&totalCount).Limit(pageSize).Offset(page * pageSize).Order("created_at desc").Find(&posts).Error
	}
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	} else {
		pageCount := int(math.Ceil(float64(totalCount) / float64(pageSize)))
		render.JSON(w, r, render.M{"items": posts, "totalCount": totalCount, "page": page, "pageSize": pageSize, "offset": page * pageSize, "pageCount": pageCount})
	}
}

func Load(id string) (post Post, err error) {

	server.DB.Where("id = ?", id).First(&post)

	if post.ID.String() == "" {
		slog.Error("post not found", slog.String("id", id))
		return post, errors.New("Post not found, check id")
	}

	return
}
