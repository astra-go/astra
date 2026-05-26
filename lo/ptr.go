package lo

// ToPtr returns a pointer to a copy of value.
//
//	p := lo.ToPtr(42)          // *int → 42
//	s := lo.ToPtr("hello")     // *string → "hello"
func ToPtr[T any](x T) *T { return &x }

// FromPtr dereferences ptr and returns the pointed-to value.
// Returns the zero value when ptr is nil.
//
//	lo.FromPtr((*int)(nil)) // 0
//	lo.FromPtr(lo.ToPtr(7)) // 7
func FromPtr[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}

// FromPtrOr dereferences ptr, returning fallback when ptr is nil.
//
//	lo.FromPtrOr((*string)(nil), "default") // "default"
func FromPtrOr[T any](ptr *T, fallback T) T {
	if ptr == nil {
		return fallback
	}
	return *ptr
}

// EmptyableToPtr returns a pointer to x, or nil when x equals the zero value.
//
//	lo.EmptyableToPtr("")  // nil
//	lo.EmptyableToPtr("x") // *string → "x"
func EmptyableToPtr[T comparable](x T) *T {
	var zero T
	if x == zero {
		return nil
	}
	return &x
}

// ToSlicePtr converts []T to []*T, where each pointer points to an independent copy.
func ToSlicePtr[T any](collection []T) []*T {
	result := make([]*T, len(collection))
	for i := range collection {
		v := collection[i]
		result[i] = &v
	}
	return result
}

// FromSlicePtr converts []*T to []T, replacing nil pointers with zero values.
func FromSlicePtr[T any](collection []*T) []T {
	result := make([]T, len(collection))
	for i, p := range collection {
		if p != nil {
			result[i] = *p
		}
	}
	return result
}
