package domain

import (
	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/timeutil"
)

type ArticleStatus string

const (
	ArticleStatusDraft     ArticleStatus = "draft"
	ArticleStatusPublished ArticleStatus = "published"
	ArticleStatusArchived  ArticleStatus = "archived"
)

type Article struct {
	orm.GORMSoftDeleteModel
	Title       string        `json:"title" gorm:"not null;size:255;index"`
	Content     string        `json:"content" gorm:"not null;type:text"`
	Summary     *string       `json:"summary,omitempty" gorm:"size:500"`
	AuthorID    uint          `json:"author_id" gorm:"not null;index"`
	Author      *User         `json:"author,omitempty" gorm:"foreignKey:AuthorID"`
	Status      ArticleStatus `json:"status" gorm:"not null;default:'draft';index"`
	Tags        *string       `json:"tags,omitempty" gorm:"size:500"`
	ViewCount   int64         `json:"view_count" gorm:"default:0"`
	LikeCount   int64         `json:"like_count" gorm:"default:0"`
	Version     int           `json:"version" gorm:"not null;default:1"`
	PublishedAt *timeutil.Time `json:"published_at,omitempty"`
}

func (Article) TableName() string {
	return "articles"
}
