package server

import (
	"fmt"
	"github.com/gofrs/uuid"
	gorm_logrus "github.com/onrik/gorm-logrus"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log/slog"
	"os"
	"time"
)

var (
	DB = initDB()
)

func initDB() (db *gorm.DB) {
	var err error

	if os.Getenv("MYSQL_DSN") != "" {
		slog.Info("Connecting to DB (mysql)", slog.String("env", os.Getenv("MYSQL_DSN")))

		db, err = gorm.Open(mysql.Open(fmt.Sprintf("%s?charset=utf8mb4&parseTime=true&loc=Local", os.Getenv("MYSQL_DSN"))),
			&gorm.Config{Logger: gorm_logrus.New()})
	} else {
		slog.Info("Connecting to DB (sqlite)")
		var dbPath string

		if os.Getenv("UNIT_TESTING") != "" {
			dbPath = "./unit-test.db"
		} else {
			dbPath = "./test.db"
		}

		l2 := gorm_logrus.New()
		l2.Debug = os.Getenv("SQL_DEBUG") == "true"

		db, err = gorm.Open(sqlite.Open(dbPath),
			&gorm.Config{Logger: l2})
	}

	if err != nil {
		slog.Error("failed to connect database", slog.Any("err", err))
		panic("failed to connect database")
	}

	return
}

// Common base structure that supports uuid as ID
// Use instead of gorm.Model when using uuid for ID instead of uint
type Base struct {
	ID        uuid.UUID  `json:"id" gorm:"type:char(36);primaryKey;"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty" sql:"index"`
}

func (base *Base) BeforeCreate(tx *gorm.DB) (err error) {
	if base.ID == uuid.Nil {
		base.ID, _ = uuid.NewV4()
	}
	return
}
