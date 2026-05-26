package astra

import "github.com/astra-go/astra/contract"

// Param is a type alias for contract.PathParam.
// Using an alias (not a new type) lets BindPath pass c.params directly to
// Binder.BindPath as []contract.PathParam with a zero-copy type conversion,
// matching the framework's zero-allocation goal for path parameters.
type Param = contract.PathParam

// Params holds URL path parameters extracted by the router.
type Params []Param

// Get returns the value of the first Param with the given key.
func (ps Params) Get(name string) (string, bool) {
	for _, p := range ps {
		if p.Key == name {
			return p.Value, true
		}
	}
	return "", false
}

// ByName returns the value of the first Param with the given key, or "".
func (ps Params) ByName(name string) string {
	v, _ := ps.Get(name)
	return v
}
