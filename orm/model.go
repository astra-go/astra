package orm

import (
	"fmt"

	"github.com/google/uuid"
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

// ModelUUID is an embeddable GORM base model that uses a UUID string as the
// primary key instead of an auto-increment integer. Embed it when you need
// globally unique, opaque identifiers (e.g. public-facing APIs, distributed
// systems, or multi-tenant databases where sequential IDs leak row counts).
//
//	type Session struct {
//	    orm.ModelUUID
//	    UserID uint `json:"user_id" gorm:"not null;index"`
//	}
//
// The UUID is generated automatically in BeforeCreate if the field is empty,
// so you never need to set it manually.
type ModelUUID struct {
	ID        string        `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CreatedAt timeutil.Time `json:"created_at"`
	UpdatedAt timeutil.Time `json:"updated_at"`
}

// BeforeCreate is a GORM hook that generates a UUID for ID (if not already
// set) and initialises CreatedAt / UpdatedAt.
func (m *ModelUUID) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	now := timeutil.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	return nil
}

// BeforeUpdate is a GORM hook that refreshes UpdatedAt on every update.
func (m *ModelUUID) BeforeUpdate(_ *gorm.DB) error {
	m.UpdatedAt = timeutil.Now()
	return nil
}

// SoftDeleteModel extends [Model] with a nullable DeletedAt field for
// logical (soft) deletion.
//
// WARNING: timeutil.Time is NOT compatible with GORM's built-in soft-delete
// filtering (which requires gorm.DeletedAt). This means:
//
//   - db.Delete(&user) physically deletes the row — it does NOT set deleted_at.
//   - db.Find(&users) returns ALL rows, including soft-deleted ones.
//   - db.Unscoped() has no effect on this model.
//
// You must filter deleted records manually on every query:
//
//	db.Where("deleted_at IS NULL").Find(&users)
//
// Or define a named scope to avoid repeating the condition:
//
//	func NotDeleted(db *gorm.DB) *gorm.DB {
//	    return db.Where("deleted_at IS NULL")
//	}
//	db.Scopes(NotDeleted).Find(&users)
//
// Use [SoftDeleteModel.SoftDelete] instead of db.Delete to mark records as
// deleted without physically removing them.
//
// If you want GORM's automatic soft-delete behaviour (auto-filter, db.Delete
// support, db.Unscoped), embed [GORMSoftDeleteModel] instead.
type SoftDeleteModel struct {
	Model
	DeletedAt *timeutil.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// GORMSoftDeleteModel extends [Model] with gorm.DeletedAt, which enables
// GORM's native soft-delete behaviour:
//
//   - db.Delete(&user) sets deleted_at instead of issuing a DELETE.
//   - db.Find(&users) automatically adds WHERE deleted_at IS NULL.
//   - db.Unscoped().Find(&users) returns all rows including soft-deleted ones.
//
// Trade-off: DeletedAt is serialised as a plain RFC3339 string in JSON
// (via gorm.DeletedAt's Time field), not the custom timeutil.Time format.
// Use this type when GORM's automatic filtering is more important than
// consistent time formatting.
//
//	type Post struct {
//	    orm.GORMSoftDeleteModel
//	    Title string `json:"title"`
//	}
type GORMSoftDeleteModel struct {
	Model
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
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
