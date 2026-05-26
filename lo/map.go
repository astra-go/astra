package lo

// ─── Keys & Values ────────────────────────────────────────────────────────────

// Keys returns all keys of m as a slice. Order is not guaranteed.
//
//	lo.Keys(map[string]int{"a": 1, "b": 2}) // ["a", "b"] (any order)
func Keys[K comparable, V any](m map[K]V) []K {
	result := make([]K, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

// Values returns all values of m as a slice. Order is not guaranteed.
//
//	lo.Values(map[string]int{"a": 1, "b": 2}) // [1, 2] (any order)
func Values[K comparable, V any](m map[K]V) []V {
	result := make([]V, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}

// ─── Entry conversion ─────────────────────────────────────────────────────────

// Entries converts m into a slice of Entry{Key, Value} pairs.
// Order is not guaranteed.
//
//	pairs := lo.Entries(map[string]int{"a": 1})
//	// []Entry[string,int]{{Key:"a", Value:1}}
func Entries[K comparable, V any](m map[K]V) []Entry[K, V] {
	result := make([]Entry[K, V], 0, len(m))
	for k, v := range m {
		result = append(result, Entry[K, V]{Key: k, Value: v})
	}
	return result
}

// FromEntries constructs a map from a slice of Entry pairs.
// Duplicate keys: later entries overwrite earlier ones.
func FromEntries[K comparable, V any](entries []Entry[K, V]) map[K]V {
	result := make(map[K]V, len(entries))
	for _, e := range entries {
		result[e.Key] = e.Value
	}
	return result
}

// ─── Map transformation ───────────────────────────────────────────────────────

// MapKeys transforms the keys of m using iteratee, preserving values.
//
//	lo.MapKeys(map[int]string{1:"a"}, func(_ string, k int) string { return fmt.Sprintf("key%d", k) })
//	// map["key1":"a"]
func MapKeys[K comparable, V any, R comparable](m map[K]V, iteratee func(value V, key K) R) map[R]V {
	result := make(map[R]V, len(m))
	for k, v := range m {
		result[iteratee(v, k)] = v
	}
	return result
}

// MapValues transforms the values of m using iteratee, preserving keys.
//
//	doubled := lo.MapValues(map[string]int{"a": 1, "b": 2}, func(v int, _ string) int { return v * 2 })
//	// map["a":2 "b":4]
func MapValues[K comparable, V, R any](m map[K]V, iteratee func(value V, key K) R) map[K]R {
	result := make(map[K]R, len(m))
	for k, v := range m {
		result[k] = iteratee(v, k)
	}
	return result
}

// ─── Filtering ────────────────────────────────────────────────────────────────

// PickBy returns a new map containing only entries for which predicate returns true.
//
//	lo.PickBy(m, func(v int, _ string) bool { return v > 0 })
func PickBy[K comparable, V any](m map[K]V, predicate func(value V, key K) bool) map[K]V {
	result := make(map[K]V)
	for k, v := range m {
		if predicate(v, k) {
			result[k] = v
		}
	}
	return result
}

// OmitBy returns a new map containing only entries for which predicate returns false.
//
//	lo.OmitBy(m, func(v int, _ string) bool { return v < 0 }) // drop negatives
func OmitBy[K comparable, V any](m map[K]V, predicate func(value V, key K) bool) map[K]V {
	return PickBy(m, func(v V, k K) bool { return !predicate(v, k) })
}

// ─── Structural helpers ───────────────────────────────────────────────────────

// Invert swaps keys and values. When values are duplicated, the last key wins.
//
//	lo.Invert(map[string]int{"a":1,"b":2}) // map[int]string{1:"a", 2:"b"}
func Invert[K comparable, V comparable](m map[K]V) map[V]K {
	result := make(map[V]K, len(m))
	for k, v := range m {
		result[v] = k
	}
	return result
}

// Assign merges all src maps into a new map.
// Later maps overwrite keys set by earlier ones.
//
//	merged := lo.Assign(defaults, overrides)
func Assign[K comparable, V any](maps ...map[K]V) map[K]V {
	total := 0
	for _, m := range maps {
		total += len(m)
	}
	result := make(map[K]V, total)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// Has reports whether key is present in m.
func Has[K comparable, V any](m map[K]V, key K) bool {
	_, ok := m[key]
	return ok
}

// MapToSlice converts a map to a slice by applying transform to each key-value pair.
// Order is not guaranteed.
//
//	lo.MapToSlice(counts, func(k string, v int) string { return fmt.Sprintf("%s=%d", k, v) })
func MapToSlice[K comparable, V, R any](m map[K]V, transform func(key K, value V) R) []R {
	result := make([]R, 0, len(m))
	for k, v := range m {
		result = append(result, transform(k, v))
	}
	return result
}
