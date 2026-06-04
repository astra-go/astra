package domain

import (
	"github.com/astra-go/astra/orm"
)

type Comment struct {
	orm.GORMSoftDeleteModel
	ArticleID uint   `json:"article_id" gorm:"not null;index"`
	UserID    uint   `json:"user_id" gorm:"not null;index"`
	User      *User  `json:"user,omitempty" gorm:"foreignKey:UserID"`
	ParentID  *uint  `json:"parent_id,omitempty" gorm:"index"`
	Content   string `json:"content" gorm:"not null;type:text"`
	LikeCount int64  `json:"like_count" gorm:"default:0"`
}

func (Comment) TableName() string {
	return "comments"
}
