package main

import (
	"areo/go-chat-backend/account"
	"areo/go-chat-backend/content"
	"areo/go-chat-backend/email"
	"areo/go-chat-backend/media"
	"areo/go-chat-backend/messaging"
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/users"
	"areo/go-chat-backend/utils"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/oauth"
	"github.com/go-chi/render"
	_ "github.com/mattn/go-sqlite3"
	"github.com/oxtoacart/bpool"
	slogchi "github.com/samber/slog-chi"
	"github.com/zknill/slogmw"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// go-chi links
//
// https://thedevelopercafe.com/articles/restful-routing-with-chi-in-go-d05a2f952b3d
//
// https://go-chi.io/#/pages/middleware

var (
	oauthSecret string
	bufpool     *bpool.BufferPool
	templates   map[string]*template.Template
)

// for debug, will need authentication eventually
func EnvHandler(w http.ResponseWriter, r *http.Request) {
	res, err := json.Marshal(os.Environ())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	_, err = w.Write(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// checks for bearer token and puts the currently logged in user into gin context
// func loadAuthenticatedUserMiddleware() gin.HandlerFunc {

/*func Verify(ja *JWTAuth, findTokenFns ...func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		hfn := func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			token, err := VerifyRequest(ja, r, findTokenFns...)
			ctx = NewContext(ctx, token, err)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(hfn)
	}
}*/

//func Authorize(secretKey string, formatter TokenSecureFormatter) func(next http.Handler) http.Handler {
//	return NewBearerAuthentication(secretKey, formatter).Authorize
//}

func loadAuthenticatedUserMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var user users.User

		slog.Debug("context", slog.Any("context", r.Context()))
		credential := r.Context().Value(oauth.CredentialContext) //.(string) // TODO, CHECK
		slog.Debug("loading user", slog.Any("credentials", credential))
		if credential == "" {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		userEmail := fmt.Sprintf("%v", credential)
		user, err := users.Load(userEmail)

		if err != nil {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, "loggedInUser", user)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func main() {

	logger := initEnvironment()

	users.InitSchema()
	content.InitSchema()
	media.InitSchema()

	email.InitEmail()

	r := chi.NewRouter()

	config := slogchi.Config{
		DefaultLevel:     slog.LevelDebug,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,
	}

	r.Use(slogchi.NewWithConfig(logger, config))

	// Note that we cannot have both
	//
	// Access-Control-Allow-Credentials: true
	// Access-Control-Allow-Origin: *
	//
	// This will be blocked by browsers! If setting either of these, the other must be turned off
	//
	r.Use(cors.Handler(cors.Options{
		//AllowedOrigins: []string{"http://localho.st:3001", "https://localho.st:3001", "http://localho.st:8080", "https://localho.st:8080", "http://localho.st:8081", "http://localho.st", "https://localho.st"},
		AllowedOrigins: []string{"http://localho.st:8080", "http://localho.st:8081"},
		// AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "DNT", "User-Agent", "X-Requested-With", "If-Modified-Since", "Cache-Control", "Range", "Access-Control-Request-Headers", "Access-Control-Request-Method", "Origin", "Priority"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	setupAPIRoutes(r)

	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "3001"
	}

	if os.Getenv("SSL_CERT") != "" && os.Getenv("SSL_KEY") != "" {
		slog.Info("Launching API HTTPS", slog.String("apiPort", apiPort))
		go http.ListenAndServeTLS(":"+apiPort, os.Getenv("SSL_CERT"), os.Getenv("SSL_KEY"), r)
	} else {
		slog.Info("Launching API HTTP", slog.String("apiPort", apiPort))
		go http.ListenAndServe(":"+apiPort, r)
	}

	hostPort := os.Getenv("HOST_PORT")
	if hostPort == "" {
		hostPort = "8080"
	}

	r2 := chi.NewRouter()
	config2 := slogchi.Config{
		DefaultLevel:     slog.LevelDebug,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,
	}
	r2.Use(slogchi.NewWithConfig(logger, config2))

	r2.Get("/", account.HomePage)
	r2.Get("/signin", account.SigninPage)

	slog.Info("Launching Host HTTP", slog.String("hostPort", apiPort))
	err := http.ListenAndServe(":"+hostPort, r2)
	if err != nil {
		panic(err)
	}
}

func setupAPIRoutes(router *chi.Mux) {

	router.Route("/api/v1", func(api chi.Router) {

		api.Use(render.SetContentType(render.ContentTypeJSON))

		api.Post("/register", account.Register)
		api.Post("/resetpassword", account.ResetPassword)
		//api.Post("/verify-registration", signup.VerifyRegistration) // not currently used, we use an oauth2 endpoint instead

		api.Group(func(authorized chi.Router) {
			authorized.Use(oauth.Authorize(oauthSecret, nil))
			authorized.Use(loadAuthenticatedUserMiddleware) //, ginBodyLogMiddleware)

			authorized.Post("/invite", account.Invite)

			//authorized.Get("/env", EnvHandler)

			authorized.Post("/media", media.PostMediaHandle)

			authorized.Get("/users", users.GetUsers)
			authorized.Get("/users/{id}", users.GetUser)
			authorized.Post("/users/{id}", users.PostUser)

			authorized.Get("/user", users.GetActiveUser)
			authorized.Post("/user", users.PostActiveUser)
			authorized.Post("/user/settings", users.PostActiveUserSettings)

			authorized.Post("/user/image", media.PostActiveUserImageHandle)
			authorized.Get("/user/image", media.GetActiveUserImage)

			authorized.Post("/user/keyword", users.SaveKeyword)
			authorized.Get("/user/keywords", users.GetKeywords)

			authorized.Get("/user/byemail/{email}", users.GetUserByEmail)
			//authorized.Get("/users", users.ActiveUsers)

			authorized.Get("/channels", content.GetChannels)
			authorized.Get("/channels/{id}", content.GetChannel)
			authorized.Get("/channels/{id}/messages", content.GetChannelMessages)

			authorized.Get("/messages", content.GetMessages)

			authorized.Get("/messages/tags", content.GetTags)

			authorized.Post("/admin/reindex", content.Reindex)
			authorized.Get("/admin/search", content.Search)
			authorized.Post("/admin/stats", content.Stats)

			authorized.Get("/messages/{id}", content.GetPrivateMessages) // might need to change this url
			authorized.Get("/message/{id}", content.GetMessage)
			authorized.Get("/messages/search", content.SearchMessages)
			authorized.Get("/channels/search", content.SearchChannels)

			authorized.Get("/chats", content.GetChats2)

			authorized.Post("/channels/{id}/subscribe", content.SubscribeToChannel)
			authorized.Get("/channels/subscriptions", content.GetSubscriptions)
			authorized.Get("/channels/suggestions", content.GetSuggestions)
			authorized.Post("/channels/{id}", content.SaveChannel)
			authorized.Post("/channels/{id}/read", content.ReadChannel)
			authorized.Delete("/channels/{id}", content.DeleteChannel)

			authorized.Post("/messages/{id}/read", content.ReadPosting)
			authorized.Post("/messages/{id}/like", content.LikePosting)

			authorized.Get("/posts", content.GetPosts)
			authorized.Get("/posts/{id}", content.GetPost)
			authorized.Post("/posts/{id}", content.SavePost)

			authorized.Post("/preview", utils.GetUrlData)

			//authorized.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			//	render.PlainText(w, r, "OK")
			//})
		})
		api.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			render.PlainText(w, r, "OK")
		})
	})
	router.Get("/images/{id}", media.GetImage)
	router.Head("/images/{id}", media.CheckImage)

	registerAuth(router)

	messaging.Configure(router)
}

func registerAuth(router *chi.Mux) {

	s := oauth.NewBearerServer(
		oauthSecret,
		time.Second*3600, // 1 hour session time
		&account.TestUserVerifier{},
		nil)

	router.Post("/oauth2/token", s.UserCredentials)
	router.Post("/oauth2/tokenRefresh", s.UserCredentials)
	router.Post("/oauth2/verify", s.ClientCredentials)
}

func initEnvironment() *slog.Logger {

	// Structured logging  https://betterstack.com/community/guides/logging/logging-in-go/
	opts := &slog.HandlerOptions{Level: slog.LevelDebug,
		ReplaceAttr: slogmw.FormatChain(slogmw.FormatTime(slog.TimeKey, time.DateTime)),
	}
	if os.Getenv("ENV") == "production" {
		opts = &slog.HandlerOptions{Level: slog.LevelWarn}
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, opts))
	slog.SetDefault(logger)

	oauthSecret = os.Getenv("OAUTH_SECRET")
	if oauthSecret == "" {
		oauthSecret = "thisISmeantTObeSECURE"
	}

	if os.Getenv("UNIT_TESTING") != "" {
		// Drop tables
		//db.DropTableIfExists(&OAUTHUser{}, "oauth_users")
		// Append Users
		//db.DropTableIfExists(&User{}, Groups{}, Orders{})
	}

	if os.Getenv("MYSQL_DSN") == "" {
		slog.Info("Checking schema in DB (sqlite)")
		//db.CreateTable(&User{})
	}

	//db.CreateTable(&User{}, Groups{}, Orders{})
	//db.AutoMigrate(&User{}, Groups{}, Orders{})

	if os.Getenv("CLIENT_ID") != "" && os.Getenv("CLIENT_SECRET") != "" {

		slog.Info("Querying user", slog.String("Client_ID", os.Getenv("CLIENT_ID")))
		var user users.User
		server.DB.Where("email = ?", os.Getenv("CLIENT_ID")).First(&user)

		// If no user exists, add.
		if user.ID.String() == "" {
			slog.Info("Querying new user", slog.String("Client_ID", os.Getenv("CLIENT_ID")))
			user.Email = os.Getenv("CLIENT_ID")

			passBcrypt, err := bcrypt.GenerateFromPassword([]byte(os.Getenv("CLIENT_SECRET")), 8)

			if err != nil {
				slog.Warn("Bcrypt failed", slog.Any("error", err))
			}

			// Insert the bcrypt password
			user.Password = string(passBcrypt)

			// Toggle the isAdmin user mode on
			//user.Groups = []Groups{Groups{ZoneName: "isAdmin"}}

			//user.AddManual()
			//db.Create(&defaultUser)
		}

	}
	return logger
}
