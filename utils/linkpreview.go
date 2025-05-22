package utils

import (
	"github.com/badoux/goscraper"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

type Preview struct {
	Url         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Images      []string `json:"images"`
}

func GetUrlData(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		slog.Error("Unable to parse form request", slog.Any("err", err))
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"message": "Unable to parse form request"})
		return
	}
	url := r.Form.Get("url")
	if url == "" {
		slog.Error("Unable to get url from form request", slog.Any("err", err))
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"message": "Unable to get url from form request"})
		return
	}
	s, err := goscraper.Scrape(url, 5)
	if err != nil {
		slog.Error("unable to preview url", slog.Any("err", err))
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"message": "Unable to preview url"})
		return
	}
	var pvw Preview
	pvw.Url = s.Preview.Link
	pvw.Title = s.Preview.Title
	pvw.Description = s.Preview.Description
	pvw.Images = s.Preview.Images

	render.JSON(w, r, pvw)
}
