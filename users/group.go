package users

import (
	"areo/go-chat-backend/server"
	"github.com/anuragkumar19/binding"
	"github.com/go-chi/render"
	"github.com/gofrs/uuid"
	"log/slog"
	"net/http"
	"time"
)

type Group struct {
	server.Base
	Name        string    `json:"name" gorm:"type:varchar(255);"`
	Description string    `json:"description"  gorm:"type:varchar(255);"`
	Imageref    string    `json:"imageref,omitempty" gorm:"type:char(36);"`
	Members     []UserRef `json:"members" gorm:"foreignkey:ID;"`
	Pending     []UserRef `json:"pending,omitempty" gorm:"foreignkey:ID;"`
	Moderator   string    `json:"moderator" gorm:"moderator"`
}

func GetGroup(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(r.URL.Query().Get("id"))
	slog.Debug("looking for group", slog.String("ID", id.String()))
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var group Group
	err = server.DB.Where("id = ?", id.String()).First(&group).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	slog.Debug("found", slog.String("group", group.ID.String()))
	render.JSON(w, r, group)
}

type GroupInterface map[string]interface{}

func PostGroup(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(r.URL.Query().Get("id"))
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var groupMod GroupInterface
	if err := binding.Bind(r, &groupMod); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	// disallow certain user fields
	delete(groupMod, "moderator")

	var group Group
	err = server.DB.Where("id = ?", id.String()).First(&group).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	err = server.DB.Model(&group).Updates(groupMod).Error
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	render.JSON(w, r, group)
}

func FindGroups(w http.ResponseWriter, r *http.Request) {

	groups := []Group{}
	err := server.DB.Where("name is not null").Limit(50).Order("last_login desc").Find(&groups).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	} else {
		render.JSON(w, r, groups)
	}

	return
}

func (group Group) New(w http.ResponseWriter, r *http.Request) {

	if err := binding.Bind(r, &group); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"ErrorMessage": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	// Check if this email address is already registered
	var count int64
	server.DB.Table("group").Where("name = ?", group.Name).Count(&count)

	if count > 0 {
		time.Sleep(time.Second * 3)
		render.Status(r, http.StatusConflict)
		render.JSON(w, r, render.M{"ErrorMessage": "Group name already exists", "status": "fail"})
		return
	}

	err := server.DB.Create(&group).Error
	if err != nil {
		slog.Error("unable to create group", "name", group.Name, "err", err)
	}

	if group.ID.String() == "" {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"ErrorMessage": "Could not create group", "status": "fail"})
	} else {
		render.Status(r, http.StatusCreated)
		render.JSON(w, r, render.M{"status": "OK", "id": group.ID})
	}
}
