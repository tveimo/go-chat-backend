package users

import (
	"areo/go-chat-backend/server"
	"areo/go-chat-backend/utils"
	"encoding/json"
	"errors"
	"github.com/anuragkumar19/binding"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/gofrs/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"log/slog"
	"math"
	"net/http"
	"os"
	"time"
)

type User struct {
	server.Base
	Email       string `json:"email" gorm:"type:varchar(255);"`
	Password    string `json:"-" gorm:"type:varchar(255);"` // don't return
	Title       string `json:"title,omitempty" gorm:"type:varchar(255);"`
	GivenName   string `json:"givenName" gorm:"type:varchar(255);"`
	MiddleName  string `json:"middleName,omitempty" gorm:"type:varchar(255);"`
	SurName     string `json:"surName" gorm:"type:varchar(255);"`
	Description string `json:"description" gorm:"type:varchar(1024);"`
	CompanyName string `json:"companyName,omitempty" gorm:"type:varchar(255)"`
	CompanyURL  string `json:"companyUrl,omitempty" gorm:"type:varchar(255)"`
	Location    string `json:"location" gorm:"type:varchar(255)"`
	Phone       string `json:"phone" gorm:"type:varchar(36);"`
	Imageref    string `json:"imageref,omitempty" gorm:"type:char(36);"`
	DOB         string `json:"DOB,omitempty" gorm:"type:varchar(16);"`
	//PreferredContactMethod string `json:"preferredContact"`
	//NotificationPreference string `json:"notificationPreference"`

	Keywords []Keyword `json:"keywords" gorm:"foreignKey:UserID"`

	ContactPhone  bool `json:"contactPhone" gorm:"type:boolean"`
	ContactEmail  bool `json:"contactEmail" gorm:"type:boolean"`
	NotifyMessage bool `json:"notifyMessage" gorm:"type:boolean"`
	NotifyPost    bool `json:"notifyPost" gorm:"type:boolean"`
	NotifyReply   bool `json:"notifyReply" gorm:"type:boolean"`

	PreferredLocale string `json:"locale" gorm:"type:varchar(10)"`

	IPAddress string     `json:"ipAddress,omitempty" gorm:"type:varchar(255);"`
	LastLogin *time.Time `json:"lastLogin,omitempty"`

	Endpoint string `json:"endpoint" gorm:"varchar(255)"`
	Auth     string `json:"auth" gorm:"varchar(255)"`
	P256dh   string `json:"p256dh" gorm:"varchar(255)"`
}

func (u User) GetTitle() string {
	if u.SurName == "" && u.GivenName == "" {
		if u.ContactEmail {
			return u.Email
		} else {
			return u.ID.String()
		}
	} else {
		if u.SurName == "" || u.GivenName == "" {
			return u.GivenName + u.SurName
		}
		return u.GivenName + " " + u.SurName
	}
}

type Keyword struct {
	server.Base
	Tag    string    `json:"tag"`
	UserID uuid.UUID `json:"-"`
}

func (k *Keyword) UnmarshalJSON(p []byte) error {
	var tmp string
	if err := json.Unmarshal(p, &tmp); err != nil {
		slog.Error("unable to unmarshal keyword", slog.Any("err", err))
		return err
	}
	k.Tag = tmp
	return nil
}

func (k *Keyword) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.Tag)
}

type UserRef struct {
	server.Base
	UserID uuid.UUID `json:"id" gorm:"type:char(36);"`
}

func GetActiveUser(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("loggedInUser").(User)
	render.JSON(w, r, user)
}

func GetUser(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var user User
	err = server.DB.Where("id = ?", id.String()).First(&user).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	render.JSON(w, r, user)
}

func GetUserByEmail(w http.ResponseWriter, r *http.Request) {
	email := chi.URLParam(r, "email")

	var user User
	err := server.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}
	render.JSON(w, r, user)
}

type UserInterface map[string]interface{}

func PostActiveUser(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("loggedInUser").(User)
	updateUser(user, w, r)
}

func PostUser(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.FromString(chi.URLParam(r, "id"))
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var user User
	err = server.DB.Where("id = ?", id.String()).First(&user).Error
	if err != nil {
		slog.Error("unable to find user", slog.Any("err", err))
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
	}

	updateUser(user, w, r)
}

func updateUser(user User, w http.ResponseWriter, r *http.Request) {

	var userMod User
	if err := binding.Bind(r, &userMod); err != nil {
		slog.Error("unable to bind json", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	// disallow certain user fields
	userMod.Password = ""
	userMod.ID = uuid.Nil
	userMod.LastLogin = nil
	userMod.Email = ""
	//delete(userMod, "LastLogin")
	//delete(userMod, "Email")
	//delete(userMod, "Password")
	//delete(userMod, "ID")
	//
	//delete(userMod, "Imageref")
	//delete(userMod, "Title")
	//delete(userMod, "SurName")
	//delete(userMod, "GivenName")
	//delete(userMod, "MiddleName")

	err := server.DB.Model(&user).Updates(userMod).Error
	//err = database.DB.Model(&user).Updates(map[string]interface{}{}).Error
	if err != nil {
		slog.Error("unable to save user", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	render.JSON(w, r, user)
}

func PostActiveUserSettings(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(User)

	type UserSettings struct {
		ContactPhone    bool   `json:"contactPhone"`
		ContactEmail    bool   `json:"contactEmail"`
		NotifyMessage   bool   `json:"notifyMessage"`
		NotifyPost      bool   `json:"notifyPost"`
		NotifyReply     bool   `json:"notifyReply"`
		PreferredLocale string `json:"locale"`
	}

	var settings UserSettings
	if err := binding.Bind(r, &settings); err != nil {
		slog.Error("unable to bind json", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	if err := server.DB.Model(&user).Updates(
		map[string]interface{}{
			"ContactPhone":    settings.ContactPhone,
			"ContactEmail":    settings.ContactEmail,
			"NotifyMessage":   settings.NotifyMessage,
			"NotifyPost":      settings.NotifyPost,
			"NotifyReply":     settings.NotifyReply,
			"PreferredLocale": settings.PreferredLocale,
		}).Error; err != nil {
		slog.Error("unable to store user settings", slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}
	render.JSON(w, r, user)
}

func ActiveUsers(w http.ResponseWriter, r *http.Request) {
	type ActiveUser struct {
		ID        uuid.UUID  `json:"id"`
		Email     string     `json:"email"`
		LastLogin *time.Time `json:"lastLogin"`
	}

	users := []User{}
	err := server.DB.Where("last_login is not null").Limit(50).Order("last_login desc").Find(&users).Error
	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
	} else {
		usersList := make([]ActiveUser, len(users))
		for i, v := range users {
			usersList[i] = ActiveUser{}
			usersList[i].Email = v.Email
			usersList[i].ID = v.ID
			usersList[i].LastLogin = v.LastLogin
		}
		render.JSON(w, r, usersList)
	}

	return
}

func abort(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("aborting", slog.Any("err", err))
	http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusForbidden)
	render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
}

func GetUsers(w http.ResponseWriter, r *http.Request) {

	users := []User{}
	query := r.URL.Query().Get("query")
	page := utils.DefaultQuery(r, "page", 0)
	pageSize := utils.DefaultQuery(r, "pageSize", 30)

	var totalCount int64
	var err error
	if query != "" {
		err = server.DB.Model(&users).Where("sur_name LIKE ? or given_name LIKE ?", "%"+query+"%", "%"+query+"%").Count(&totalCount).Limit(pageSize).Offset(page * pageSize).Order("created_at desc").Find(&users).Error
	} else {
		err = server.DB.Model(&users).Where("created_at is not null").Count(&totalCount).Limit(pageSize).Offset(page * pageSize).Order("created_at desc").Find(&users).Error
	}
	if err != nil {
		abort(w, r, err)
		return
	} else {
		pageCount := int(math.Ceil(float64(totalCount) / float64(pageSize)))
		render.JSON(w, r, render.M{"items": users, "totalCount": totalCount, "page": page, "pageSize": pageSize, "offset": page * pageSize, "pageCount": pageCount})
	}
}

func Load(email string) (user User, err error) {

	server.DB.Where("email = ?", email).First(&user)

	//logger.Debugln("loaded user, email;", user.Email, "userID;", user.ID)

	if user.ID.String() == "" {
		slog.Error("user not found", slog.String("email", email))
		return user, errors.New("User not found, check authentication")
	}

	return
}

func EncryptPassword(password string) (cryptpassword string) {
	pass, _ := bcrypt.GenerateFromPassword([]byte(password), 8)
	return string(pass)
}

func (user User) New(w http.ResponseWriter, r *http.Request) {

	if err := binding.Bind(r, &user); err != nil {
		render.Status(r, http.StatusForbidden)
		render.JSON(w, r, render.M{"ErrorMessage": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	// Check if this email address is already registered
	var count int64
	server.DB.Table("users").Where("email = ?", user.Email).Count(&count)

	if count > 0 {
		time.Sleep(time.Second * 3)

		render.Status(r, http.StatusConflict)
		render.JSON(w, r, render.M{"ErrorMessage": "User already exists", "status": "fail"})
		return
	}

	passBcrypt, err := bcrypt.GenerateFromPassword([]byte(user.Password), 8)

	if err != nil {
		slog.Error("Bcrypt failed", slog.Any("err", err))
	}

	// Insert the bcrypt password
	user.Password = string(passBcrypt)

	// Add the users IP
	user.IPAddress = utils.GetIPAdress(r)

	err = server.DB.Create(&user).Error
	if err != nil {
		slog.Error("unable to create user", slog.String("email", user.Email))
	}

	// send welcome email

	if user.ID.String() == "" {
		render.JSON(w, r, render.M{"ErrorMessage": "Could not create user", "status": "fail"})
	} else {
		render.Status(r, http.StatusCreated)
		render.JSON(w, r, render.M{"status": "OK", "id": user.ID})
	}
}

func NewSignupOrSignin(email, password, ipAddress string) (user User, err error) {

	// Check if this email address is already registered
	var count int64
	err = server.DB.Table("users").Where("email = ?", email).First(&user).Count(&count).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		slog.Error("unable to check if user already exist", slog.String("email", email), slog.Any("err", err))
		return user, err
	}

	if count > 0 {
		/* we allow signin, but won't update password */
		//var userMod User
		//userMod.Password = EncryptPassword(password)
		//userMod.IPAddress = ipAddress
		//log.Printf("updating user with new password: %v", userMod)
		//
		//err = server.DB.Model(&user).Updates(userMod).Error
		//if err != nil {
		//	logger.Errorf("unable to save user: %v", err)
		//	return user, err
		//}

	} else {
		user.Email = email
		user.Password = EncryptPassword(password)
		user.IPAddress = ipAddress
		err = server.DB.Create(&user).Error
		if err != nil {
			slog.Error("unable to create user", slog.String("email", user.Email), slog.Any("err", err))
			return user, err
		}
	}
	// send welcome email
	//signup.SengWelcomeMail(email)
	return user, nil
}

func SaveKeyword(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(User)

	type param struct {
		Keyword string `json:"keyword" binding:""`
		Add     bool   `json:"add" binding:""`
	}

	var keywordParams *param
	var err error

	if err = binding.Bind(r, &keywordParams); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, render.M{"status": "fail",
			"err": err.Error() + " - Check JSON body input is not malformed"})
		return
	}

	keyword := Keyword{
		Tag:    keywordParams.Keyword,
		UserID: user.ID,
	}

	if keywordParams.Add {
		err = server.DB.Model(&keyword).FirstOrCreate(&keyword, Keyword{UserID: user.ID, Tag: keywordParams.Keyword}).Error
	} else {
		err = server.DB.Where("user_id = ? AND name = ?", user.ID, keywordParams.Keyword).Delete(&keyword).Error
	}

	if err != nil {
		slog.Error("unable to store keyword", slog.String("keyword", keywordParams.Keyword),
			slog.Bool("param", keywordParams.Add), slog.Any("err", err))
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, render.M{"status": "fail", "err": err.Error()})
		return
	}

	render.JSON(w, r, keyword)
}

func GetKeywords(w http.ResponseWriter, r *http.Request) {

	user := r.Context().Value("loggedInUser").(User)

	var kws []Keyword

	err := server.DB.Model(&kws).Where("user_id = ?", user.ID).Find(&kws).Error

	if err != nil {
		render.Status(r, http.StatusNotFound)
		render.JSON(w, r, render.M{"status": "error", "err": err.Error()})
		return
	}

	var keywords []string = make([]string, len(kws))
	for i, v := range kws {
		keywords[i] = v.Tag
	}
	render.JSON(w, r, render.M{"data": keywords})
}

func InitSchema() {

	slog.Info("setting up User schema")
	server.DB.AutoMigrate(&User{})

	slog.Info("setting up Group schema")
	server.DB.AutoMigrate(&Group{})

	// https://gorm.io/docs/migration.html
	server.DB.AutoMigrate(&Keyword{})
	server.DB.AutoMigrate(&UserRef{})

	if os.Getenv("CLIENT_ID") != "" && os.Getenv("CLIENT_SECRET") != "" {

		slog.Info("Querying user", slog.String("email", os.Getenv("CLIENT_ID")))
		var defaultUser User
		server.DB.Where("email = ?", os.Getenv("CLIENT_ID")).First(&defaultUser)
		if defaultUser.Email == "" {
			slog.Info("Creating unit test user", slog.String("Client_ID", os.Getenv("CLIENT_ID")))
			defaultUser.Email = os.Getenv("CLIENT_ID")

			passBcrypt, err := bcrypt.GenerateFromPassword([]byte(os.Getenv("CLIENT_SECRET")), 8)

			if err != nil {
				slog.Warn("Bcrypt failed", slog.Any("error", err))
			}

			defaultUser.Password = string(passBcrypt)

			err = server.DB.Create(&defaultUser).Error
			if err != nil {
				slog.Error("unable to create unit test user", slog.String("email", defaultUser.Email), slog.Any("err", err))
			}
		}
	}
}
