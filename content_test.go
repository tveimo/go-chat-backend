package main

import (
	"areo/go-chat-backend/content"
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

var TestChannels = []content.Channel{
	{Title: "global channel", Description: "this is the first channel"},
	{Title: "local channel", Description: "channel room for developers"},
}

// test new user registration
func TestNewChannels(t *testing.T) {

	// run these tests with the unittest user
	if os.Getenv("CLIENT_ID") == "" || os.Getenv("CLIENT_SECRET") == "" {
		slog.Debug("need to run NewChannels tests with CLIENT_ID & CLIENT_SECRET specified in environment")
		return
	}

	for _, v := range TestChannels {

		// sign in as unittest user
		oauthToken, err := OAUTHsignin(os.Getenv("CLIENT_ID"), os.Getenv("CLIENT_SECRET"))
		assert.NoError(t, err, "unable to authenticate")

		type Resp struct {
			Status string `json:"status"`
			ID     uint   `json:"id"`
		}
		jsonValue, _ := json.Marshal(v)

		slog.Debug("adding", slog.String("channel", string(jsonValue)))

		// try without authenticating
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/channels/new", bytes.NewBuffer(jsonValue))

		req.Header.Set("Authorization", oauthToken)
		req.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(w, req)

		assert.Equal(t, 201, w.Code)

		var respChannel content.Channel
		decoder := json.NewDecoder(w.Body)
		slog.Debug("response", slog.String("body", string(w.Body.Bytes())))

		err = decoder.Decode(&respChannel)
		assert.NoError(t, err, "unable to create channel")

		assert.Equal(t, 201, w.Code)
		assert.Equal(t, v.Title, respChannel.Title)
	}
}
