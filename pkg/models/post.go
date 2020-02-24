package models

import (
	"fmt"
	"time"
	"strings"

	"github.com/qor/l10n"
	"github.com/qor/sorting"
	"github.com/qor/validations"
	"github.com/qor/publish2"

	"github.com/gosimple/slug"
	"github.com/jinzhu/gorm"
	"github.com/qor/media"
	"github.com/qor/media/media_library"
	"github.com/qor/media/oss"
)

type Category struct {
	l10n.Locale
	sorting.Sorting
	ID    uint `gorm:"primary_key"`
	Name  string
	Posts []Post `gorm:"many2many:category_post"`
	l10n.LocaleCreatable
}

func (category Category) Validate(db *gorm.DB) {
	if strings.TrimSpace(category.Name) == "" {
		db.AddError(validations.NewError(category, "Name", "Name can not be empty"))
	}
}

/*
type Category struct {
	gorm.Model
	l10n.Locale
	sorting.Sorting
	Name string
	Code string

	Categories []Category
	CategoryID uint
}

func (category Category) Validate(db *gorm.DB) {
	if strings.TrimSpace(category.Name) == "" {
		db.AddError(validations.NewError(category, "Name", "Name can not be empty"))
	}
}

func (category Category) DefaultPath() string {
	if len(category.Code) > 0 {
		return fmt.Sprintf("/category/%s", category.Code)
	}
	return "/"
}
*/

type Tag struct {
	ID    uint `gorm:"primary_key"`
	Name  string
	Posts []Post `gorm:"many2many:tag_post"`
	l10n.LocaleCreatable
}

type Link struct {
	gorm.Model
	Url      string
	Title    string
	ImageUrl string
	PostID   uint
}

type Video struct {
	gorm.Model
	Url         string
	Value       string `gorm:"type:longtext"`
	Description string `gorm:"type:longtext"`
	PostID      uint
}

type Image struct {
	gorm.Model
	File   media_library.MediaLibraryStorage `gorm:"type:longtext" sql:"size:4294967295;" media_library:"url:/content/{{class}}/{{primary_key}}/{{column}}.{{extension}};path:./public"`
	PostID uint
}

func (Image) GetSizes() map[string]*media.Size {
	return map[string]*media.Size{
		"small":           {Width: 320, Height: 320},
		"middle":          {Width: 640, Height: 640},
		"big":             {Width: 1024, Height: 720},
		"article_preview": {Width: 390, Height: 300},
		"preview":         {Width: 200, Height: 200},
	}
}

type Document struct {
	gorm.Model
	File   oss.OSS `gorm:"type:longtext" sql:"size:4294967295;" media_library:"url:/public/content/publications/{{basename}}.{{extension}};path:./public"`
	PostID uint
}

type Post struct {
	ID         uint       `gorm:"primary_key"`
	Categories []Category `gorm:"many2many:category_post"`
	Tags       []Tag      `gorm:"many2many:tag_post"`
	Title      string
	Slug       string `gorm:"unique"`
	Body       string `gorm:"type:longtext"`
	Summary    string `gorm:"type:longtext"`
	Images     []Image
	Documents  []Document
	Videos     []Video
	Links      []Link
	Type       string
	Created    int32
	Updated    int32

	publish2.Version
	publish2.Schedule
	publish2.Visible
}

type Page struct {
	ID         uint       `gorm:"primary_key"`
	Categories []Category `gorm:"many2many:category_page"`
	Tags       []Tag      `gorm:"many2many:tag_page"`
	Title      string
	Slug       string `gorm:"unique"`
	Body       string `gorm:"type:longtext"`
	Summary    string `gorm:"type:longtext"`
	Images     []Image
	Documents  []Document
	Videos     []Video
	Links      []Link
	Type       string
	Created    int32
	Updated    int32
}

type Event struct {
	ID         uint       `gorm:"primary_key"`
	Categories []Category `gorm:"many2many:category_post"`
	Title      string
	Slug       string `gorm:"unique"`
	Body       string `gorm:"type:longtext"`
	Summary    string `gorm:"type:longtext"`
	Images     []Image
	Documents  []Document
	Videos     []Video
	Links      []Link
	Type       string
	Created    int32
	Updated    int32
	StartDate  int32
	EndDate    int32
}

func (p *Post) BeforeCreate() (err error) {
	if p.Created == 0 {
		p.Created = int32(time.Now().Unix())
	}

	if p.Updated == 0 {
		p.Updated = int32(time.Now().Unix())
	}

	// p.Slug = createUniqueSlug(p.Title)

	fmt.Printf("======> New post: %#v", p.Title)
	fmt.Printf("======> New post: %#v", p.Summary)
	fmt.Printf("======> New post: %#v", p.Images)
	if len(p.Images) > 0 {
		fmt.Printf("=======> Images: %#v", p.Images[0].File.FileName)
	}
	for i := range p.Images {
		p.Images[i].File.Sizes = p.Images[i].GetSizes()
		file, _ := p.Images[i].File.Base.FileHeader.Open()
		p.Images[i].File.Scan(file)
	}

	return nil
}

func createUniqueSlug(title string) string {
	slugTitle := slug.Make(title)
	if len(slugTitle) > 50 {
		slugTitle = slugTitle[:50]
		if slugTitle[len(slugTitle)-1:] == "-" {
			slugTitle = slugTitle[:len(slugTitle)-1]
		}
	}
	return slugTitle
}
