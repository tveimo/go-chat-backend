package email

import (
	"areo/go-chat-backend/users"
	"bytes"
	"encoding/base64"
	"github.com/go-gomail/gomail"
	"html/template"
	"io/ioutil"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// This address must be verified with Amazon SES, if using that service
	Sender = "test@areo.no"
)

var (
	OverrideRecipient = "test@areo.net.au"
)

func InitEmail() {

	if os.Getenv("OVERRIDE_EMAIL") == "false" {
		OverrideRecipient = ""
	}
}

// credentials https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html

func SendRegistrationEmail(user *users.User, email, token string) error {

	path, _ := filepath.Abs("emailtemplates")
	files, _ := ioutil.ReadDir("emailtemplates")
	for _, file := range files {
		slog.Debug("templates", slog.String("file", filepath.Join(path, file.Name())))
	}
	emailTemplatePath := "emailtemplates/signup.html"
	emailTemplateName := "signup.html"
	emailSubject := "Account Registration"

	if user != nil {
		slog.Debug("using invite email since sending on behalf of", slog.String("userID", user.ID.String()))
		emailTemplatePath = "emailtemplates/invite.html"
		emailTemplateName = "invite.html"
		emailSubject = "Invitation to Networking Group"
	}

	t := template.New(emailTemplatePath)

	var err error
	t, err = t.ParseFiles(emailTemplatePath)
	if err != nil {
		slog.Error("unable to parse email template", slog.Any("err", err))
	}

	serverHost := os.Getenv("SERVER_HOST")

	if serverHost == "" {
		serverHost = "http://localhost:8080"
	}

	base64Text := base64.StdEncoding.EncodeToString([]byte(email))

	data := map[string]interface{}{
		"Email":        email,
		"EmailEncoded": base64Text,
		"URL":          template.URL(serverHost + "/#/signup/verify/" + token),
		"user":         user,
	}

	var tpl bytes.Buffer
	if err := t.ExecuteTemplate(&tpl, emailTemplateName, data); err != nil {
		slog.Error("unable to parse email template", slog.Any("err", err))
		return err
	}
	slog.Debug("email", slog.String("content", string(tpl.Bytes())))
	SendEmail2(email, emailSubject, string(tpl.Bytes()), user == nil)
	return nil
}

func SendPasswordResetEmail(email, token string) error {

	path, _ := filepath.Abs("emailtemplates")
	files, _ := ioutil.ReadDir("emailtemplates")
	for _, file := range files {
		slog.Debug("templates file", slog.String("filename", filepath.Join(path, file.Name())))
	}
	t := template.New("emailtemplates/reset.html")

	var err error
	t, err = t.ParseFiles("emailtemplates/reset.html")
	if err != nil {
		slog.Error("unable to parse email template", slog.Any("err", err))
	}

	serverHost := os.Getenv("SERVER_HOST")

	if serverHost == "" {
		serverHost = "http://localhost:8080"
	}

	data := map[string]interface{}{
		"Email": email,
		"URL":   template.URL(serverHost + "/#/signup/reset/" + token),
	}

	var tpl bytes.Buffer
	if err := t.ExecuteTemplate(&tpl, "reset.html", data); err != nil {
		slog.Error("unable to execute email template", slog.Any("err", err))
		return err
	}
	slog.Debug("email", slog.String("content", string(tpl.Bytes())))
	SendEmail2(email, "Reset passord verifisering", string(tpl.Bytes()), false)
	return nil
}

func SendEmail2(recipient, subject, content string, approved bool) error {

	var ccEmail string

	if approved {
		// approved email domains
		if strings.HasSuffix(recipient, "@areo.net.au") ||
			strings.HasSuffix(recipient, "@gmail.com") {
			ccEmail = "test@areo.net.au"
		} else {
			slog.Debug("address is not approved", slog.String("email", recipient))
			recipient = "test@areo.net.au"
			return nil
		}
	}

	m := gomail.NewMessage()
	m.SetHeader("From", Sender)
	if OverrideRecipient != "" {
		m.SetHeader("To", OverrideRecipient)
	} else {
		m.SetHeader("To", recipient)
	}
	if ccEmail != "" {
		m.SetAddressHeader("Cc", ccEmail, "Joe Test")
	}

	m.SetHeader("Subject", subject)
	m.SetBody("text/html", content)

	// TODO use env varibles for the smtp host and authentication details
	d := gomail.NewDialer("email-smtp.eu-west-1.amazonaws.com", 587, "xxx", "yyy")

	if err := d.DialAndSend(m); err != nil {
		slog.Error("unable to send email to recipient", slog.String("email", recipient), slog.Any("err", err))
		return err
	}
	return nil
}
