package main

import (
	"areo/go-chat-backend/account"
	"areo/go-chat-backend/content"
	"areo/go-chat-backend/media"
	"areo/go-chat-backend/users"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// run with UNIT_TESTING=true go test -v

// common data for testing

var router *chi.Mux

var TestUsers = []users.User{
	{Email: "test+unittesting1@areo.no", Password: "abcxyz231", GivenName: "John", SurName: "Doe"},
	{Email: "test+unittesting2@areo.no", Password: "abcxyz231", GivenName: "John 2", SurName: "Doe"},
	{Email: "test+unittesting3@areo.no", Password: "abcxyz231", GivenName: "Jane 3", SurName: "Doe"},
	{Email: "test+unittesting4@areo.no", Password: "abcxyz231", GivenName: "Jane 4", SurName: "Doe"},
	{Email: "test+unittesting5@areo.no", Password: "abcxyz231", GivenName: "Jane 5", SurName: "Doe"},
}

// This method signs in the user at https://hostname/oauth2/token and returns the bearer token to
// use with subsequent calls.
func OAUTHsignin(id, secret string) (oauthToken string, err error) {

	type Resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}

	w := httptest.NewRecorder()

	formData := url.Values{
		"username":   {id},
		"password":   {secret},
		"grant_type": {"password"},
	}

	req, _ := http.NewRequest("POST", "/oauth2/token", strings.NewReader(formData.Encode()))

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(formData.Encode())))

	router.ServeHTTP(w, req)

	var respJSON Resp
	decoder := json.NewDecoder(w.Body)
	slog.Debug("response", slog.String("body", w.Body.String()))

	err = decoder.Decode(&respJSON)
	if err != nil {
		slog.Error("unable to decode json", slog.Any("err", err))
		return "", err
	}

	if respJSON.AccessToken == "" {
		return oauthToken, errors.New("access_token not found")
	}

	return fmt.Sprintf("Bearer %s", respJSON.AccessToken), err
}

// This method verifies the signup token at https://hostname/oauth2/verify and returns the bearer token to
// use with subsequent calls.
func OAUTHverify(registrationToken string) (oauthToken string, err error) {

	type Resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}

	w := httptest.NewRecorder()

	formData := url.Values{
		"client_id":     {"go-chat-backend"},
		"client_secret": {registrationToken},
		"grant_type":    {"authorization_code"},
		"scope":         {"user.read user.write profile"},
	}

	req, _ := http.NewRequest("POST", "/oauth2/verify", strings.NewReader(formData.Encode()))

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(formData.Encode())))

	router.ServeHTTP(w, req)

	var respJSON Resp
	decoder := json.NewDecoder(w.Body)
	slog.Debug("response", slog.String("body", w.Body.String()))

	err = decoder.Decode(&respJSON)
	if err != nil {
		return "", err
	}

	if respJSON.AccessToken == "" {
		return oauthToken, errors.New("access_token not found")
	}

	return fmt.Sprintf("Bearer %s", respJSON.AccessToken), err
}

func TestMain(m *testing.M) {

	oauthSecret = os.Getenv("OAUTH_SECRET")
	if oauthSecret == "" {
		oauthSecret = "testingSECUREsetup"
	}

	_ = initEnvironment()

	users.InitSchema()
	content.InitSchema()
	media.InitSchema()

	router = chi.NewMux()

	setupAPIRoutes(router)

	code := m.Run()

	// should clear out unit-test.sqlite before exiting
	os.Exit(code)
}

// test new user registration
func TestNewUsers(t *testing.T) {

	for k, v := range TestUsers {

		w := httptest.NewRecorder()
		req := createUserRequest(v.Email, v.Password)

		router.ServeHTTP(w, req)

		type Resp struct {
			Status string `json:"status"`
		}

		var respJSON Resp
		decoder := json.NewDecoder(w.Body)
		slog.Debug("response", slog.String("body", w.Body.String()))

		err := decoder.Decode(&respJSON)
		if err != nil {
			t.Log(err)

			assert.Nil(t, err)
		}

		//assert.NotEqual(t, respJSON.ID, uuid.Nil) // ID should not be zero

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "OK", respJSON.Status)

		// Next, verify registration by using generated token, this should sign us in

		registration, err := account.GetLastSignupToken()
		assert.NoError(t, err, "missing registration token, is UNIT_TESTING environment specified")

		oauthToken, err := OAUTHverify(registration.VerificationToken)
		assert.NoError(t, err, "unable to verify registration token")

		// fetch current user record to retrieve user id

		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/user", nil)

		req.Header.Set("Authorization", oauthToken)

		router.ServeHTTP(w, req)

		var respUser users.User
		decoder = json.NewDecoder(w.Body)
		slog.Debug("response", slog.String("body", w.Body.String()))

		err = decoder.Decode(&respUser)
		assert.NoError(t, err, "unable to fetch current user")

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, v.Email, respUser.Email)
		// update our users slice with registered ID
		TestUsers[k].ID = respUser.ID
	}

}

// test user updates
// registration only stores username and password, now we store names
func TestUpdateUsers(t *testing.T) {
	for _, v := range TestUsers {

		// Update user details

		type Resp struct {
			Status string `json:"status"`
			ID     uint   `json:"id"`
		}

		//var respJSON Resp

		jsonValue, _ := json.Marshal(v)

		slog.Debug("updating user with data", slog.String("user data", string(jsonValue)))

		// try without authenticating
		w := httptest.NewRecorder()
		req, err := http.NewRequest("POST", "/api/v1/user", bytes.NewBuffer(jsonValue))

		if err != nil {
			slog.Error("unable to create request", slog.Any("err", err))
		}
		req.Header.Set("Content-Type", "application/json")

		//log.Printf("using headers: %v", req.Header.Values("Authorization"))
		router.ServeHTTP(w, req)

		assert.Equal(t, 401, w.Code)

	}
}

// Test invalid bearer token
func TestBadAuth(t *testing.T) {

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/user", nil)

	req.Header.Set("Authorization", "Bearer XXX")

	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// register new user by passing in an email address only
func createUserRequest(email string, password string) *http.Request {

	registration := account.Registration{
		Email:    email,
		Password: password,
	}

	jsonValue, _ := json.Marshal(registration)

	slog.Debug("creating user with data", slog.String("user data", string(jsonValue)))
	// signup requires no prior authentication
	req, err := http.NewRequest("POST", "/api/v1/register", bytes.NewBuffer(jsonValue))

	if err != nil {
		slog.Error("unable to create request", slog.Any("err", err))
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}
