package models

import (
	"github.com/jinzhu/gorm"
)

/*
type Tag struct {
	gorm.Model
	Name string `gorm:"size:32;unique" json:"name" yaml:"name"`
}
*/

type Service struct {
	gorm.Model
	Name        string       `json:"name" yaml:"name"`
	Slug        string       `json:"slug,omitempty" yaml:"slug,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	URLs        []*URL       `json:"urls,omitempty" yaml:"urls,omitempty"`
	PublicKeys  []*PublicKey `json:"public_keys,omitempty" yaml:"public_keys,omitempty"`
	Tags        []*Tag       `gorm:"many2many:service_tags;" json:"tags,omitempty" yaml:"tags,omitempty"`
}

type URL struct {
	gorm.Model
	Name      string `gorm:"size:255;unique" json:"href" yaml:"href"`
	Healthy   bool   `json:"healthy" yaml:"healthy"`
	ServiceID uint   `json:"-" yaml:"-"`
}

type PublicKey struct {
	gorm.Model
	UID         string `gorm:"primary_key" json:"id,omitempty" yaml:"id,omitempty"`
	UserID      string `json:"user_id,omitempty" yaml:"user_id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Value       string `json:"value" yaml:"value"`
	ServiceID   uint   `json:"-" yaml:"-"`
}
