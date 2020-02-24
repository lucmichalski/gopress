package services

import (
	"fmt"
	"os"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"

	"github.com/lucmichalski/gopress/pkg/models"
)

var DB *gorm.DB

var (
	host     = getEnvOrDefault("DBHOST", "127.0.0.1")
	port     = getEnvOrDefault("DBPORT", "3306")
	user     = getEnvOrDefault("DBUSER", "root")
	password = getEnvOrDefault("DBPASSWORD", "aado33ve79T!")
	dbname   = getEnvOrDefault("DBNAME", "homef")
)

func Init() *gorm.DB {
	mysqlString := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local", user, password, host, port, dbname)

	//psqlInfo := fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=disable", host, port, user, dbname, password)
	db, err := gorm.Open("mysql", mysqlString)
	if err != nil {
		panic(err)
	}
	db.LogMode(true)
	DB = db

	var post models.Post
	var video []models.Video
	var image []models.Image
	var link []models.Link
	var documents []models.Document

	DB.AutoMigrate(&models.Event{}, &models.Category{}, &models.Post{}, &models.Document{}, &models.Video{}, &models.Image{}, &models.Link{})

	DB.Model(&post).Related(&video)
	DB.Model(&post).Related(&image)
	DB.Model(&post).Related(&link)
	DB.Model(&post).Related(&documents)
	return DB
}

func GetDB() *gorm.DB {
	return DB
}

func getEnvOrDefault(variable string, defaultValue string) string {
	thevar := os.Getenv(variable)

	if len(thevar) > 0 {
		return thevar
	}
	return defaultValue
}
