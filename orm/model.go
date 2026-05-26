package orm

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/astra-go/astra/timeutil"
)

// Model is an embeddable GORM base model that uses [timeutil.Time] for
// CreatedAt and UpdatedAt. Embed it in your GORM models instead of gorm.Model
// when you want consistent time formatting across JSON responses and the database.
//
//	type User struct {
//	    orm.Model
//	    Name string `json:"name" gorm:"not null"`
//	}
//
// GORM's built-in autoCreateTime/autoUpdateTime only work with time.Time and
// integer fields. BeforeCreate and BeforeUpdate hooks are used here instead.
type Model struct {
	ID        uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt timeutil.Time `json:"created_at"`
	UpdatedAt timeutil.Time `json:"updated_at"`
}

// BeforeCreate is a GORM hook that sets CreatedAt (if not already set) and
// always refreshes UpdatedAt before a record is inserted.
func (m *Model) BeforeCreate(_ *gorm.DB) error {
	now := timeutil.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	return nil
}

// BeforeUpdate is a GORM hook that refreshes UpdatedAt on every update.
func (m *Model) BeforeUpdate(_ *gorm.DB) error {
	m.UpdatedAt = timeutil.Now()
	return nil
}

// SoftDeleteModel extends [Model] with a nullable DeletedAt field for
// logical (soft) deletion.
//
// Important: timeutil.Time is not compatible with GORM's built-in soft-delete
// filtering (which requires gorm.DeletedAt). You must add WHERE conditions
// manually or use a named scope:
//
//	db.Where("deleted_at IS NULL").Find(&users)
//
// Use [SoftDeleteModel.SoftDelete] instead of db.Delete to mark records as
// deleted without physically removing them.
type SoftDeleteModel struct {
	Model
	DeletedAt *timeutil.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// SoftDelete marks the record as deleted by setting DeletedAt to now and
// persisting the change via an UPDATE statement. Pass the actual concrete
// model (e.g. &post) so GORM resolves the correct table name.
//
//	var p Post
//	db.First(&p, id)
//	p.SoftDelete(db, &p)
func (m *SoftDeleteModel) SoftDelete(db *gorm.DB, model any) error {
	if db == nil {
		return fmt.Errorf("orm: SoftDelete called with nil db")
	}
	now := timeutil.Now()
	m.DeletedAt = &now
	return db.Model(model).Update("deleted_at", m.DeletedAt).Error
}
