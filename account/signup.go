package account

import (
	"areo/go-chat-backend/email"
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/users"
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/anuragkumar19/binding"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-chi/render"
	"golang.org/x/crypto/nacl/secretbox"
	"io"
	"io/ioutil"
	"log/slog"
	"math/big"
	"strconv"
	"strings"

	"net/http"
	"os"
	"time"
)

type Registration struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	GroupID      string `json:"groupId"`
	SessionToken string `json:"sessionToken"`
}

type RegisterToken struct {
	Email             string
	VerificationToken string
}

var (
	secret         = "6368616e676520746869732070617373776f726420746f206120736563726574"
	RegisterTokens = []RegisterToken{}
)

// only used for unit testing, mocks registration emails, allowing retrieval of signup token
func GetLastSignupToken() (registration RegisterToken, err error) {
	if len(RegisterTokens) > 0 {
		registration = RegisterTokens[len(RegisterTokens)-1]
		RegisterTokens = RegisterTokens[:len(RegisterTokens)-1]
		return registration, nil
	}
	return registration, fmt.Errorf("no registrations recorded")
}

// rest endpoint for frontend
func Register(w http.ResponseWriter, r *http.Request) {

	var registerJSON Registration

	if err := binding.Bind(r, &registerJSON); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"ErrorMessage": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	if strings.TrimSpace(registerJSON.Email) == "" || strings.TrimSpace(registerJSON.Password) == "" {
		slog.Error("unable to create signup token for empty email or password", slog.String("email", registerJSON.Email))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error",
			"err": fmt.Errorf("unable to create signup token for empty email or password: #{registerJSON.Email}, aborting")})
		return
	}

	token, err := CreateSignupToken(registerJSON.Email, registerJSON.Password, "", "")
	if err != nil {
		slog.Error("unable to create signup token", slog.String("email", registerJSON.Email), slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error",
			"err": fmt.Sprintf("unable to create signup token for email: %s, aborting", registerJSON.Email)})
		return
	}
	if os.Getenv("UNIT_TESTING") != "" {
		// put token into test structure
		register := RegisterToken{
			Email:             registerJSON.Email,
			VerificationToken: token,
		}
		RegisterTokens = append(RegisterTokens, register)
	} else {
		email.SendRegistrationEmail(nil, registerJSON.Email, token)
	}

	render.JSON(w, r, render.M{"status": "OK"})
}

func Invite(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(users.User)

	slog.Debug("inviting on behalf of user", slog.String("ID", user.ID.String()))

	var registerJSON Registration

	if err := binding.Bind(r, &registerJSON); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"ErrorMessage": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	if strings.TrimSpace(registerJSON.Email) == "" {
		slog.Error("unable to create signup token for empty email", slog.String("email", registerJSON.Email))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error",
			"err": fmt.Errorf("unable to create signup token for empty email: #{registerJSON.Email}, aborting")})
		return
	}

	if strings.TrimSpace(registerJSON.Password) == "" {
		slog.Debug("creating random password for invite token")
		registerJSON.Password = "-1rnd" // should trigger random password on verification
	}

	slog.Debug("email: %s", registerJSON.Email)
	token, err := CreateSignupToken(registerJSON.Email, registerJSON.Password, registerJSON.GroupID, registerJSON.SessionToken)
	if err != nil {
		slog.Error("unable to create signup token for email", slog.String("email", registerJSON.Email), slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error",
			"err": fmt.Sprintf("unable to create signup token for email: %s, aborting", registerJSON.Email)})
		return
	}
	if os.Getenv("UNIT_TESTING") != "" {
		// put token into test structure
		register := RegisterToken{
			Email:             registerJSON.Email,
			VerificationToken: token,
		}
		RegisterTokens = append(RegisterTokens, register)
	} else {
		email.SendRegistrationEmail(&user, registerJSON.Email, token)
	}

	render.JSON(w, r, render.M{"status": "OK"})
}

// rest endpoint for frontend
func ResetPassword(w http.ResponseWriter, r *http.Request) {

	var registerJSON Registration

	if err := binding.Bind(r, &registerJSON); err != nil {
		//if err := json.NewDecoder(r.Body).Decode(&registerJSON); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"ErrorMessage": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	if strings.TrimSpace(registerJSON.Email) == "" {
		slog.Error("unable to reset password for empty email", slog.String("email", registerJSON.Email))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error",
			"err": fmt.Errorf("unable to reset password empty email: #{registerJSON.Email}, aborting")})
		return
	}
	token, err := CreateResetPasswordToken(registerJSON.Email)
	if err != nil {
		slog.Error("unable to reset password", slog.String("email", registerJSON.Email), slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error",
			"err": fmt.Sprintf("unable to create reset password token for email: %s, aborting", registerJSON.Email)})
		return
	}
	if os.Getenv("UNIT_TESTING") != "" {
		// put token into test structure
		register := RegisterToken{
			Email:             registerJSON.Email,
			VerificationToken: token,
		}
		RegisterTokens = append(RegisterTokens, register)
	} else {
		email.SendPasswordResetEmail(registerJSON.Email, token)
	}

	render.JSON(w, r, render.M{"status": "OK"})
}

func Signup(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"error": err.Error()})
		return
	}
	email := r.Form.Get("email")
	password := r.Form.Get("password")
	_, err = CreateSignupToken(email, password, "", "")
	if err != nil {
		slog.Error("unable to create signup token", slog.String("email", email), slog.Any("error", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error", "err": fmt.Sprintf("unable to create signup token for email: %s, aborting", email)})
		return
	}
	//email.SendRegistrationEmail(email, token)

	// use StatusMovedPermanently?
	http.Redirect(w, r, "/signup/sent", http.StatusTemporaryRedirect)
}

// rest endpoint for frontend
func VerifyRegistration(w http.ResponseWriter, r *http.Request) {

	var verifyJSON struct {
		Token string `json:"token"`
	}

	body, _ := ioutil.ReadAll(r.Body)
	err := json.NewDecoder(bytes.NewReader(body)).Decode(&verifyJSON)
	r.Body = ioutil.NopCloser(bytes.NewReader(body)) // put it back?

	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"ErrorMessage": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, render.M{"status": "OK"})
}

func Verify(w http.ResponseWriter, r *http.Request) {

	token := r.URL.Query().Get("token")

	slog.Debug("verifying", slog.String("token", token))
	email, _, groupID, _, err := VerifySignupToken(token)
	if err != nil {
		slog.Error("unable to verify signup token", slog.String("token", token), slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "error", "err": fmt.Sprintf("unable to verify signup token: %s", token)})
		return
	}
	slog.Debug("new user registration", slog.String("email", email), slog.String("groupID", groupID))
	render.JSON(w, r, render.M{"status": "OK"})
}

// Validating a signup token should result in the user being stored in the database
// and subsequent being logged in. It will return an oauth token and refresh token
// that the frontend can use.
//
// we are passed in the validation token

func ValidateSignupToken(w http.ResponseWriter, r *http.Request) {

	type validationJSON struct {
		Token     string
		IPAddress string
	}

	var validation validationJSON

	if err := binding.Bind(r, &validation); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail",
			"ErrorMessage": err.Error() + " - Check JSON body input is not malformed"})
		return
	}
	email, password, groupID, _, err := VerifySignupToken(validation.Token)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail",
			"ErrorMessage": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	slog.Info("adding verified user: %s, password: %s, groupID: %s", email, password, groupID)

	// create new user record with provided username & password
	var count int64
	err = server.DB.Find("email = ?", email).Count(&count).Error
	if err != nil {
		slog.Error("unable to query for existing users", slog.String("email", email), slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err})
		return
	} else {
		if count > 0 {
			slog.Error("user already exist in database", slog.String("email", email), slog.Any("err", err))
			render.Status(r, http.StatusConflict)
			render.JSON(w, r, render.M{"status": "fail", "": fmt.Errorf("user already exist in database, email: %s", email)})
			return
		}
	}

	/*user, err := users.NewSignup(email, password, validation.IPAddress)
	if err != nil {
		logger.Errorf("unable to create new user for email: %s; %v", email, err)
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"status": "fail", "reason": err})
	}
	c.JSON(http.StatusOK, user)*/
}

type ExpectedClaims struct {
	jwt.StandardClaims
	Email    string `json:"email"`
	Password string `json:"password"`
}

func verifySignupJWT(token string) (email, password string, err error) {

	keyBytes, err := ioutil.ReadFile(os.Getenv("CERT_PATH_PREFIX") + "signup/ssl/signup.crt.pem")
	if len(keyBytes) == 0 || err != nil {
		return email, password, errors.New("unable to read certificate for signup emails from file: " + os.Getenv("CERT_PATH_PREFIX") + "signup/ssl/signup.crt.pem")
	}
	jwtKey, err := jwt.ParseRSAPublicKeyFromPEM(keyBytes)
	if err != nil {
		return email, password, err
	}

	claims := &ExpectedClaims{}

	decoded, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		//logger.Debugln("claims:", token.Claims, "header:", token.Header)
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return jwtKey, nil
	})

	if err != nil {
		slog.Error("unable to parse signup verification token", slog.Any("err", err))
		return email, password, fmt.Errorf("unable to parse signup verification token: %s", err.Error())
	}

	if claims, ok := decoded.Claims.(*ExpectedClaims); ok && decoded.Valid {
		slog.Debug("verifying SWT", slog.String("issuer", claims.Issuer),
			slog.String("email", claims.Email), slog.Int64("expiry", claims.ExpiresAt))

		if time.Now().Unix() > claims.ExpiresAt {
			slog.Error("signup verification token has expired")
		} else {
			email = claims.Email
			password = claims.Password
		}
	}
	return
}

// generate random key

func GenerateKey() (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"
	ret := make([]byte, 32)
	for i := 0; i < 32; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret = append(ret, letters[num.Int64()])
	}

	return string(ret), nil
}

// https://godoc.org/golang.org/x/crypto/nacl/secretbox

func CreateSignupToken(email, password, groupID, sessionToken string) (ret string, err error) {
	secretKeyBytes, err := hex.DecodeString(secret)
	if err != nil {
		return ret, err
	}

	var secretKey [32]byte
	copy(secretKey[:], secretKeyBytes)

	buf := bytes.Buffer{}
	err = gob.NewEncoder(&buf).Encode([]string{
		email,
		password,
		strconv.FormatInt(time.Now().Add(time.Duration(24)*time.Hour).Unix(), 10),
		groupID,
		sessionToken,
	})
	if err != nil {
		return ret, err
	}

	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return ret, err
	}

	encrypted := secretbox.Seal(nonce[:], buf.Bytes(), &nonce, &secretKey)
	ret = base32.StdEncoding.EncodeToString(encrypted)
	slog.Info("signup token", slog.Int("length", len(ret)), slog.Int("input length", len(email)+len(password)+10))
	return
}

func CreateResetPasswordToken(email string) (ret string, err error) {
	secretKeyBytes, err := hex.DecodeString(secret)
	if err != nil {
		return ret, err
	}

	var secretKey [32]byte
	copy(secretKey[:], secretKeyBytes)

	buf := bytes.Buffer{}
	err = gob.NewEncoder(&buf).Encode([]string{
		email, strconv.FormatInt(time.Now().Add(time.Duration(24)*time.Hour).Unix(), 10)})
	if err != nil {
		return ret, err
	}

	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return ret, err
	}

	encrypted := secretbox.Seal(nonce[:], buf.Bytes(), &nonce, &secretKey)
	ret = base32.StdEncoding.EncodeToString(encrypted)
	slog.Info("signup token", slog.Int("length", len(ret)), slog.Int("input length", len(email)+10))
	return
}

// When you decrypt, you must use the same nonce and key you used to
// encrypt the message. One way to achieve this is to store the nonce
// alongside the encrypted message. Above, we stored the nonce in the first
// 24 bytes of the encrypted text. Since it's fixed length we don't need to use
// gob to encode it.

func VerifySignupToken(token string) (email, password, groupID, sessionToken string, err error) {

	encrypted, err := base32.StdEncoding.DecodeString(token)
	if err != nil {
		return email, password, groupID, sessionToken, err
	}

	secretKeyBytes, err := hex.DecodeString(secret)
	if err != nil {
		return email, password, groupID, sessionToken, err
	}

	var secretKey [32]byte
	copy(secretKey[:], secretKeyBytes)

	var decryptNonce [24]byte
	copy(decryptNonce[:], encrypted[:24])
	decrypted, ok := secretbox.Open(nil, encrypted[24:], &decryptNonce, &secretKey)
	if !ok {
		return email, password, groupID, sessionToken, fmt.Errorf("unable to verify signup token")
	}

	if len(decrypted) > 10 {

		decoder := gob.NewDecoder(bytes.NewReader(decrypted))
		var entry []string
		decoder.Decode(&entry)

		expiry, _ := strconv.ParseInt(entry[2], 10, 64)
		expiryAt := time.Unix(expiry, 0)
		email = entry[0]
		password = entry[1]
		groupID = entry[2]
		sessionToken = entry[3]

		if time.Now().Unix() > expiryAt.Unix() {
			slog.Error("signup verification token has expired")
		}
		return email, password, groupID, sessionToken, nil
	}
	return email, password, groupID, sessionToken, fmt.Errorf("unable to verify signup token")
}
