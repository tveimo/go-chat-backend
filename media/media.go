package media

import (
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/users"
	"bytes"
	"errors"
	"github.com/anuragkumar19/binding"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/gofrs/uuid"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Media struct {
	server.Base
	OwnerID      uuid.UUID `json:"ownerID" gorm:"type:char(36);column:user_foreignKey;not null;"`
	OriginalName string    `json:"originalName"`
	Size         uint64    `json:"size"`
	ContentType  string    `json:"contentType"`
}

func PostMediaHandle(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	var mediaHandle Media
	if err := binding.Bind(r, &mediaHandle); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}
	mediaHandle.OwnerID = user.ID

	err := server.DB.Create(&mediaHandle).Error
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	slog.Debug("saved media handle", slog.String("id", mediaHandle.ID.String()))

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, mediaHandle)
}

func PostActiveUserImageHandle(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	slog.Debug("checking if request contains file")

	r.ParseMultipartForm(32 << 20) // limit your max input length, 32MB
	file, fi, err := r.FormFile("file")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if err != nil {
		slog.Error("unable to extract image", slog.Any("err", err))
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"message": "No file is received"})
		return
	}
	slog.Debug("uploaded", slog.Any("file", fi.Filename))
	fileName := user.ID.String() //+ extension

	// The file is received, so let's save it
	dst, err := os.OpenFile(contentDirectory()+fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)

	if err != nil {
		slog.Error("unable to save file", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"message": "Unable to save the file"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"message": "Unable to save the file"})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, render.M{"message": "Uploaded"})
}

func GetActiveUserImage(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	if file, err := os.ReadFile(contentDirectory() + user.ID.String()); err != nil {
		slog.Debug("serving static image as fallback")

		serveImage(w, r, user)
	} else {
		w.Header().Set("Content-Type", "image/png")
		_, err = w.Write(file)
		if err != nil {
			slog.Error("unable to write image", slog.Any("err", err))
		}
	}
}

// writeImage encodes an image 'img' in jpeg format and writes it into ResponseWriter.
func writeImage(w http.ResponseWriter, img *image.Image) {

	buffer := new(bytes.Buffer)
	if err := jpeg.Encode(buffer, *img, nil); err != nil {
		slog.Debug("unable to encode image", slog.Any("err", err))
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(buffer.Bytes())))
	if _, err := w.Write(buffer.Bytes()); err != nil {
		slog.Debug("unable to write image", slog.Any("err", err))
	}
}

func contentDirectory() (directory string) {
	var ok bool
	if directory, ok = os.LookupEnv("CONTENT_DIR"); !ok {
		directory = "~/content/go-chat-backend/images/"
	}
	slog.Debug("using content directory", slog.String("path", directory))
	return
}

func CheckImage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	if _, err := os.Stat(contentDirectory() + id.String()); errors.Is(err, os.ErrNotExist) {
		render.Status(r, http.StatusNotFound)
		return
	}
	render.Status(r, http.StatusOK)
}

func GetImage(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	// look up using wildcard for extension?
	// https://www.socketloop.com/tutorials/golang-use-wildcard-patterns-with-filepath-glob-example

	if file, err := os.ReadFile(contentDirectory() + id.String()); err != nil {

		var user users.User
		err = server.DB.Where("id = ?", id.String()).First(&user).Error
		if err != nil {
			render.Status(r, http.StatusNotFound)
			render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
			return
		}

		serveImage(w, r, user)

	} else {
		//defer file.Close()
		w.Header().Set("Content-Type", "image/png")
		_, err = w.Write(file)
		if err != nil {
			slog.Error("unable to write image", slog.Any("err", err))
		}
	}
}

func serveImage(w http.ResponseWriter, r *http.Request, user users.User) {
	initials := ""
	one := strings.TrimSpace(user.GivenName)
	two := strings.TrimSpace(user.SurName)
	if one == "" && two == "" {
		initials = user.Email[0:2]
	} else if one == "" {
		initials = two[0:2]
	} else {
		initials = one[0:2]
	}

	// return svg image
	svg := "<!DOCTYPE svg PUBLIC \"-//W3C//DTD SVG 1.1//EN\" \"http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd\">" +
		"<svg class='sub-av' height='40px' width='40px' xmlns='http://www.w3.org/2000/svg' viewBox='0 0 40 40'>" +
		"<circle cx='50%' cy='50%' r='50%' class='circle' fill='#aaa'></circle><text fill='#eee' text-anchor='middle' " +
		"font-family='sans-serif' font-weight='bold' dominant-baseline='central' alignment-baseline='central' " +
		"text-transform='uppercase' x='50%' y='50%' class='text'>" +
		initials + "</text></svg>"

	w.Header().Set("Content-Type", "image/svg+xml")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(svg))
	if err != nil {
		slog.Error("unable to write image", slog.Any("err", err))
	}
}

func InitSchema() {

	slog.Info("setting up Media schema")

	// Users schema
	//database.DB.CreateTable(&Media{})
	server.DB.AutoMigrate(&Media{})
}
