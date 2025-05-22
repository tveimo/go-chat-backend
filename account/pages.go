package account

import (
	"bytes"
	"github.com/go-chi/render"
	"html/template"
	"log/slog"
	"net/http"
	"os"
)

func HomePage(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, r, "home.html", "Home")
}

func SigninPage(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, r, "signin.html", "Signin")
}

func SignupPage(w http.ResponseWriter, r *http.Request) {
	renderHTML(w, r, "register.html", "Signup")
}

func renderHTML(w http.ResponseWriter, r *http.Request, templateName string, title string) {

	staticPaths := "pages/head.html"
	t := template.New(templateName)

	var err error
	t, err = t.ParseFiles("pages/"+templateName, staticPaths)
	if err != nil {
		slog.Error("unable to parse page template", slog.String("templateName", templateName),
			slog.Any("err", err))
		abortHTML(w, r, http.StatusInternalServerError, err)
	}

	serverHost := os.Getenv("SERVER_HOST")

	if serverHost == "" {
		serverHost = "http://localhost:8080"
	}

	data := map[string]interface{}{
		"title":      title,
		"serverHost": serverHost,
	}

	var tpl bytes.Buffer
	if err := t.ExecuteTemplate(&tpl, templateName, data); err != nil {
		slog.Error("unable to parse page template", slog.Any("err", err))
		abortHTML(w, r, http.StatusInternalServerError, err)
	}
	slog.Debug("rendering page template", slog.String("content", string(tpl.Bytes())))

	render.HTML(w, r, tpl.String())
}

func abortHTML(w http.ResponseWriter, r *http.Request, status int, err error) {
	render.Status(r, status)
	render.HTML(w, r, "unable to render pages, please try again later")
}
