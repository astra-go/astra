package domain

import (
	"github.com/astra-go/astra/orm"
)

type User struct {
	orm.Model
	Username     string  `json:"username" gorm:"uniqueIndex;not null;size:50"`
	Email        string  `json:"email" gorm:"uniqueIndex;not null;size:100"`
	PasswordHash string  `json:"-" gorm:"not null"`
	Role         string  `json:"role" gorm:"not null;default:'reader'"` // admin, author, reader
	Bio          *string `json:"bio,omitempty" gorm:"type:text"`
	Avatar       *string `json:"avatar,omitempty" gorm:"size:255"`
}

func (User) TableName() string {
	return "users"
}
