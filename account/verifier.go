package account

import (
	"areo/go-chat-backend/content"
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/users"
	"errors"
	"github.com/go-chi/oauth"
	"github.com/gofrs/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"log/slog"
	"net/http"
	"time"
)

type TestUserVerifier struct {
}

var (
	oauthSecret string
)

// ValidateUser validates username and password returning an error if the user credentials are wrong
func (*TestUserVerifier) ValidateUser(username, password, scope string, req *http.Request) error {

	slog.Info("validating user", slog.String("username", username))
	// return nil // if testing, to allow any login

	if username == "" || password == "" {
		return errors.New("Authentication failed - Wrong username / password")
	}

	// Query if the user is available
	var user users.User
	var count int64
	err := server.DB.Where("email = ?", username).First(&user).Count(&count).Error
	if err == gorm.ErrRecordNotFound {
		slog.Error("unable to find user in db", slog.String("username", username), slog.Any("err", err))
		return err
	} else if err != nil {
		slog.Error("unable to query for user in db", slog.String("username", username), slog.Any("err", err))
		return err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))

	if err == nil {

	}
	// If the user exists but the password didn't match, it results in an error
	// Count will show that the email is already in the database
	if err != nil && count > 0 && user.Password != "" {
		return errors.New("Authentication failed - Wrong username / password 0x343")
	}

	if err == nil {
		err = server.DB.Model(&user).Where("email = ?", username).Update("LastLogin", time.Now()).Error
		if err != nil {
			slog.Error("unable to save lastLogin", slog.String("username", username), slog.Any("err", err))
		}
	}

	return nil
}

func (*TestUserVerifier) ValidateClient(clientID, clientSecret, scope string, req *http.Request) error {

	slog.Info("Validating client", slog.String("clientID", clientID))

	email, password, _, _, err := VerifySignupToken(clientSecret)
	if err != nil {
		slog.Error("unable to verify signup", slog.String("token", clientSecret))
		return err
	}

	user, err := users.NewSignupOrSignin(email, password, "")
	if err != nil {
		slog.Error("unable to sign up new user", slog.Any("err", err))
		return err
	} else {
		slog.Debug("new user registration", slog.String("email", user.Email))
	}

	return nil
}

// AddClaims provides additional claims to the token
// These are available server side only, when the token is decrypted
func (*TestUserVerifier) AddClaims(tokenType oauth.TokenType, credential, tokenID, scope string, req *http.Request) (map[string]string, error) {
	claims := make(map[string]string)
	return claims, nil
}

// StoreTokenId saves the token Id generated for the user
func (*TestUserVerifier) StoreTokenID(tokenType oauth.TokenType, credential, tokenID, refreshTokenID string) error {

	slog.Info("StoreTokenId", slog.String("credential", credential), slog.String("tokenID", tokenID),
		slog.String("refreshTokenID", refreshTokenID), slog.String("tokenType", string(tokenType)))

	return nil
}

// AddProperties provides additional information to the token response
// These are not encrypted, so can be read by frontend js code
func (*TestUserVerifier) AddProperties(tokenType oauth.TokenType, credential, tokenID, scope string, req *http.Request) (map[string]string, error) {
	props := make(map[string]string)

	slog.Debug("adding properties to token", slog.String("credential", credential),
		slog.String("tokenID", tokenID), slog.String("tokenType", string(tokenType)), slog.String("scope", scope))

	props["uid"] = credential
	var user users.User
	err := server.DB.Where("email = ?", credential).First(&user).Error
	if err != nil {
		slog.Error("unable to look up user by email", slog.Any("err", err))
		return props, err
	}
	props["id"] = user.ID.String()
	props["ts"] = time.Now().Format(time.RFC3339)

	slog.Debug("properties", slog.Any("props", props))
	/*	_, err := Load(credential)
		if err != nil {
			logger.Errorf("unable to lookup just logged in user: %s; %v", credential, err)
		} else {
			// if user is admin, add props["admin"] = "true"
			logger.Infof("adding properties for user: %s; %v", credential, props)
		}*/

	//logger.Printf("AddProperties %s %s %s", credential, tokenID, tokenType)
	return props, nil
}

// ValidateTokenId validates token Id
func (*TestUserVerifier) ValidateTokenID(tokenType oauth.TokenType, credential, tokenID, refreshTokenID string) error {

	slog.Info("ValidateTokenId", slog.String("credential", credential), slog.String("tokenID", tokenID),
		slog.String("refreshTokenID", refreshTokenID), slog.String("tokenType", string(tokenType)))

	return nil
}

// We use ValidateCode to verify signup token
func (*TestUserVerifier) ValidateCode(clientID, clientSecret, code, redirectURI string, req *http.Request) (userName string, err error) {

	slog.Info("ValidateTokenId", slog.String("clientID", clientID), slog.String("clientSecret", "..."),
		slog.String("code", code), slog.String("redirectURI", string(redirectURI)))

	email, password, groupID, _, err := VerifySignupToken(clientSecret)
	if err != nil {
		slog.Error("unable to verify signup token", slog.String("clientSecret", clientSecret), slog.Any("err", err))
		return userName, err
	}

	if email == "" || password == "" {
		slog.Error("unable to verify with empty email or password", slog.String("clientSecret", clientSecret), slog.Any("err", err))
		return userName, err
	}

	if password == "-1rnd" {
		slog.Debug("assigning random password for new user", slog.String("email", email))
	}

	user, err := users.NewSignupOrSignin(email, password, "")
	if err != nil {
		slog.Error("unable to sign up new user", slog.Any("err", err))
		return userName, err
	} else {
		slog.Info("new user registration", slog.String("password", "[redacted]"), slog.String("groupID",
			groupID), slog.String("email", user.Email), slog.String("groupID", groupID))
		if groupID != "" {
			slog.Info("subscribing new user to group", slog.String("groupID", groupID))
			err = SubscribeToChannelSignup(user, groupID)
			if err != nil {
				slog.Error("unable to sign up to group", slog.String("groupID", groupID), slog.Any("err", err))
				err = nil
			}
		}
	}

	return user.Email, err
}

func SubscribeToChannelSignup(user users.User, groupID string) error {

	id, err := uuid.FromString(groupID)
	if err != nil {
		slog.Error("unable to decode group ID for subscription", slog.String("groupID", groupID),
			slog.Any("err", err))
		return err
	}

	slog.Debug("subscribing to channel", slog.String("groupID", id.String()),
		slog.String("userID", user.ID.String()))

	var channel content.Channel
	err = server.DB.
		Preload("Read").
		Where("id = ?", id.String()).First(&channel).Error
	if err != nil {
		slog.Error("unable to locate channel", slog.String("id", id.String()), slog.Any("err", err))
		return err
	}
	slog.Debug("subscribing to channel", slog.String("channel", channel.Title),
		slog.String("userID", user.ID.String()))

	usermod := &users.User{}
	usermod.ID = user.ID

	var subscriber content.ChannelParticipant
	subscriber.UserID = usermod.ID
	subscriber.ChannelID = id
	subscriber.Approved = true
	subscriber.CreatedByID = user.ID

	err = server.DB.Where(
		subscriber).
		Assign(subscriber).
		FirstOrCreate(&content.ChannelParticipant{}).Error

	err = server.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "user_id"}},
		UpdateAll: true,
	}).Create(&subscriber).Error

	// add system message to channel, indicating subscribed user
	content.UserSubscribed(user, channel)

	if err != nil {
		slog.Error("unable to subscribe to channel", slog.String("channel title", channel.Title), slog.Any("err", err))
		return err
	}

	slog.Debug("subscribed to channel", slog.String("title", channel.Title), slog.String("userID", user.ID.String()))
	return nil
}
