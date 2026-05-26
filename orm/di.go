// Package orm — di.go provides helper functions that register ORM implementations
// into an Astra DI container (github.com/astra-go/astra/di).
//
// After calling ProvideRepository[T] once, any component resolved from the same
// container that depends on contract.Repository[T] will receive the GORM-backed
// implementation automatically — no manual wiring required.
//
// # Typical usage inside a Module.Install
//
//	func (m *UserModule) Install(app *astra.App) error {
//	    orm.ProvideRepository[User](m.container, m.db)
//	    orm.ProvideTxRunner(m.container, m.db)
//
//	    svc, err := di.Invoke[*UserService](m.container)
//	    if err != nil { return err }
//
//	    g := app.Group("/api/users")
//	    g.GET("",     svc.List)
//	    g.POST("",    svc.Create)
//	    return nil
//	}
//
// # Named repositories (multiple DBs / table configs)
//
//	orm.ProvideRepositoryNamed[Event](c, "analytics", analyticsDB)
//	orm.ProvideRepositoryNamed[Event](c, "primary",   primaryDB)
//
//	repo := di.MustInvokeNamed[contract.Repository[Event]](c, "analytics")
package orm

import (
	"github.com/astra-go/astra/contract"
	"github.com/astra-go/astra/di"
	"gorm.io/gorm"
)

// ProvideRepository registers a GORM-backed contract.Repository[T] provider
// in container c.
//
// The provider is a singleton: the same *Repository[T] is returned on every
// subsequent di.Invoke[contract.Repository[T]] call.
//
// Returns di.ErrDuplicate if contract.Repository[T] has already been registered.
func ProvideRepository[T any](c *di.Container, db *gorm.DB) error {
	return di.Provide[contract.Repository[T]](c, func(_ *di.Container) (contract.Repository[T], error) {
		return NewRepository[T](db), nil
	})
}

// ProvideRepositoryNamed registers a named GORM-backed contract.Repository[T]
// provider. Use this when multiple databases or table configurations share the
// same entity type and you need to distinguish them by name.
//
//	orm.ProvideRepositoryNamed[Order](c, "primary",  primaryDB)
//	orm.ProvideRepositoryNamed[Order](c, "archive",  archiveDB)
//
//	primary := di.MustInvokeNamed[contract.Repository[Order]](c, "primary")
func ProvideRepositoryNamed[T any](c *di.Container, name string, db *gorm.DB) error {
	return di.ProvideNamed[contract.Repository[T]](c, name, func(_ *di.Container) (contract.Repository[T], error) {
		return NewRepository[T](db), nil
	})
}

// ProvideTxRunner registers a GORM-backed contract.TxRunner provider in
// container c.
//
// Service-layer code that depends on contract.TxRunner will receive a
// *GORMTxRunner that writes the active transaction into the context via
// orm.WithTx, enabling automatic propagation to all Repository[T] calls
// that use orm.FromCtx.
//
// Returns di.ErrDuplicate if contract.TxRunner has already been registered.
func ProvideTxRunner(c *di.Container, db *gorm.DB) error {
	return di.Provide[contract.TxRunner](c, func(_ *di.Container) (contract.TxRunner, error) {
		return NewTxRunner(db), nil
	})
}
