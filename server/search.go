package server

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/gofrs/uuid"
	"log/slog"
	"os"
	"strings"
)

var (
	Index = initBleve()
)

func indexDirectory() (directory string) {
	var ok bool
	if directory, ok = os.LookupEnv("INDEX_DIR"); !ok {
		home, _ := os.UserHomeDir()
		directory = home + "/content/go-chat-backend/index/"
	}
	slog.Info("using index directory", slog.String("dir", directory))
	return
}

func initBleve() (index bleve.Index) {

	index, err := bleve.Open(indexDirectory() + "bleve")

	if err != nil {
		slog.Warn("unable to load index, recreating", slog.Any("err", err))
		index, err = bleve.New(indexDirectory()+"bleve", indexMappings())
		if err != nil {
			slog.Error("unable to create new index", slog.Any("err", err))
		}
	}

	return index
}

func indexMappings() *mapping.IndexMappingImpl {

	itemMapping := bleve.NewDocumentMapping()

	titleFieldMapping := bleve.NewTextFieldMapping()
	titleFieldMapping.Analyzer = "en"
	itemMapping.AddFieldMappingsAt("title", titleFieldMapping)

	descriptionFieldMapping := bleve.NewTextFieldMapping()
	descriptionFieldMapping.Analyzer = "en"
	itemMapping.AddFieldMappingsAt("description", descriptionFieldMapping)

	idMapping := bleve.NewKeywordFieldMapping()
	itemMapping.AddFieldMappingsAt("id", idMapping)

	channelIdMapping := bleve.NewKeywordFieldMapping()
	itemMapping.AddFieldMappingsAt("channelId", channelIdMapping)

	typeMapping := bleve.NewKeywordFieldMapping()
	itemMapping.AddFieldMappingsAt("type", typeMapping)

	indexMapping := bleve.NewIndexMapping()

	indexMapping.AddDocumentMapping("item", itemMapping)

	return indexMapping
}

func BleveStats() string {
	stats, err := Index.Stats().MarshalJSON()
	if err != nil {
		slog.Error("unable to fetch bleve stats", slog.Any("err", err))
	}

	return string(stats)
}

type ItemDocument struct {
	Title       string   `json:"Title"`
	Description string   `json:"description"`
	ID          string   `json:"id"`
	ChannelID   string   `json:"channelId"`
	Type        string   `json:"type"`
	Tags        []string `json:"tags"`
}

func IndexEntry(id uuid.UUID, content ItemDocument) {

	err := Index.Delete(id.String())
	if err != nil {
		slog.Error("unable to remove entry", slog.String("id", id.String()), slog.Any("err", err))
	}

	err = Index.Index(id.String(), content)

	if err != nil {
		slog.Error("unable to index data", slog.Any("err", err))
	}
}

func Search(freetext string) []string {

	query := bleve.NewMatchQuery(freetext)
	req := bleve.NewSearchRequest(query)

	res, err := Index.Search(req)
	if err != nil {
		slog.Error("unble to search index", slog.Any("err", err))
	}
	hits := res.Hits

	ids := make([]string, hits.Len())

	for _, v := range hits {
		if strings.TrimSpace(v.ID) != "" {
			ids = append(ids, v.ID)
		}
	}
	return ids
}

func SearchChannels(freetext string) []string {

	query := bleve.NewMatchQuery(freetext)
	req := bleve.NewSearchRequest(query)
	req.Fields = []string{"channelId"}
	res, err := Index.Search(req)
	if err != nil {
		slog.Error("unable to search index", slog.Any("err", err))
	}
	hits := res.Hits

	ids := make([]string, hits.Len())

	for _, v := range hits {
		_, err := Index.Document(v.ID)
		if err != nil {
			slog.Error("unable to index", slog.Any("err", err))
		}
		if strings.TrimSpace(v.ID) != "" {
			slog.Info("got channel", slog.String("id", v.Fields["channelId"].(string)))
			ids = append(ids, v.Fields["channelId"].(string))
		}
	}
	return ids
}
