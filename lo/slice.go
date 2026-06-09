package lo

import "math/rand/v2"

// ─── Transformation ───────────────────────────────────────────────────────────

// Map transforms each element of collection using iteratee, returning a new slice.
//
//	doubled := lo.Map([]int{1, 2, 3}, func(x, _ int) int { return x * 2 })
//	// [2, 4, 6]
func Map[T, R any](collection []T, iteratee func(item T, index int) R) []R {
	result := make([]R, len(collection))
	for i, v := range collection {
		result[i] = iteratee(v, i)
	}
	return result
}

// Filter returns the elements of collection for which predicate returns true.
//
//	evens := lo.Filter([]int{1, 2, 3, 4}, func(x, _ int) bool { return x%2 == 0 })
//	// [2, 4]
func Filter[V any](collection []V, predicate func(item V, index int) bool) []V {
	result := make([]V, 0, len(collection)/2)
	for i, v := range collection {
		if predicate(v, i) {
			result = append(result, v)
		}
	}
	return result
}

// Reduce reduces collection to a single value by applying accumulator left-to-right.
//
//	sum := lo.Reduce([]int{1, 2, 3, 4}, func(acc, x, _ int) int { return acc + x }, 0)
//	// 10
func Reduce[T, R any](collection []T, accumulator func(agg R, item T, index int) R, initial R) R {
	for i, v := range collection {
		initial = accumulator(initial, v, i)
	}
	return initial
}

// ReduceRight is like Reduce but iterates from the last element to the first.
func ReduceRight[T, R any](collection []T, accumulator func(agg R, item T, index int) R, initial R) R {
	for i := len(collection) - 1; i >= 0; i-- {
		initial = accumulator(initial, collection[i], i)
	}
	return initial
}

// FlatMap maps each element to a slice and concatenates the results.
//
//	lo.FlatMap([][]int{{1, 2}, {3}}, func(v []int, _ int) []int { return v })
//	// [1, 2, 3]
func FlatMap[T, R any](collection []T, iteratee func(item T, index int) []R) []R {
	result := make([]R, 0, len(collection))
	for i, v := range collection {
		result = append(result, iteratee(v, i)...)
	}
	return result
}

// ForEach iterates over collection calling iteratee for each element.
func ForEach[T any](collection []T, iteratee func(item T, index int)) {
	for i, v := range collection {
		iteratee(v, i)
	}
}

// Times generates a slice of n elements by calling iteratee with each index.
//
//	squares := lo.Times(4, func(i int) int { return i * i })
//	// [0, 1, 4, 9]
func Times[T any](count int, iteratee func(index int) T) []T {
	result := make([]T, count)
	for i := 0; i < count; i++ {
		result[i] = iteratee(i)
	}
	return result
}

// ─── Search & Membership ─────────────────────────────────────────────────────

// Contains reports whether element is present in collection.
func Contains[T comparable](collection []T, element T) bool {
	for _, v := range collection {
		if v == element {
			return true
		}
	}
	return false
}

// ContainsBy reports whether any element satisfies predicate.
func ContainsBy[T any](collection []T, predicate func(item T) bool) bool {
	for _, v := range collection {
		if predicate(v) {
			return true
		}
	}
	return false
}

// Every returns true when all elements satisfy predicate (vacuously true for empty slices).
func Every[T any](collection []T, predicate func(item T) bool) bool {
	for _, v := range collection {
		if !predicate(v) {
			return false
		}
	}
	return true
}

// Some returns true when at least one element satisfies predicate.
func Some[T any](collection []T, predicate func(item T) bool) bool {
	return ContainsBy(collection, predicate)
}

// None returns true when no element satisfies predicate.
func None[T any](collection []T, predicate func(item T) bool) bool {
	return !Some(collection, predicate)
}

// Count returns the number of occurrences of value in collection.
func Count[T comparable](collection []T, value T) int {
	n := 0
	for _, v := range collection {
		if v == value {
			n++
		}
	}
	return n
}

// CountBy returns the number of elements satisfying predicate.
func CountBy[T any](collection []T, predicate func(item T) bool) int {
	n := 0
	for _, v := range collection {
		if predicate(v) {
			n++
		}
	}
	return n
}

// Find returns the first element satisfying predicate and true.
// Returns the zero value and false when not found.
func Find[T any](collection []T, predicate func(item T) bool) (T, bool) {
	for _, v := range collection {
		if predicate(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// FindIndexOf returns the first element satisfying predicate, its index, and true.
// Returns zero value, -1, and false when not found.
func FindIndexOf[T any](collection []T, predicate func(item T) bool) (T, int, bool) {
	for i, v := range collection {
		if predicate(v) {
			return v, i, true
		}
	}
	var zero T
	return zero, -1, false
}

// IndexOf returns the first index of element in collection, or -1 if not found.
func IndexOf[T comparable](collection []T, element T) int {
	for i, v := range collection {
		if v == element {
			return i
		}
	}
	return -1
}

// LastIndexOf returns the last index of element in collection, or -1 if not found.
func LastIndexOf[T comparable](collection []T, element T) int {
	for i := len(collection) - 1; i >= 0; i-- {
		if collection[i] == element {
			return i
		}
	}
	return -1
}

// ─── Head & Tail ─────────────────────────────────────────────────────────────

// First returns the first element and true, or the zero value and false for an empty slice.
func First[T any](collection []T) (T, bool) {
	if len(collection) == 0 {
		var zero T
		return zero, false
	}
	return collection[0], true
}

// FirstOrDefault returns the first element, or defaultValue if the slice is empty.
func FirstOrDefault[T any](collection []T, defaultValue T) T {
	if v, ok := First(collection); ok {
		return v
	}
	return defaultValue
}

// Last returns the last element and true, or the zero value and false for an empty slice.
func Last[T any](collection []T) (T, bool) {
	if len(collection) == 0 {
		var zero T
		return zero, false
	}
	return collection[len(collection)-1], true
}

// LastOrDefault returns the last element, or defaultValue if the slice is empty.
func LastOrDefault[T any](collection []T, defaultValue T) T {
	if v, ok := Last(collection); ok {
		return v
	}
	return defaultValue
}

// ─── Sub-slicing ──────────────────────────────────────────────────────────────

// Take returns the first n elements. Returns the full slice if n ≥ len(collection).
func Take[T any](collection []T, n int) []T {
	if n <= 0 {
		return []T{}
	}
	if n >= len(collection) {
		result := make([]T, len(collection))
		copy(result, collection)
		return result
	}
	result := make([]T, n)
	copy(result, collection[:n])
	return result
}

// TakeRight returns the last n elements. Returns the full slice if n ≥ len(collection).
func TakeRight[T any](collection []T, n int) []T {
	if n <= 0 {
		return []T{}
	}
	if n >= len(collection) {
		result := make([]T, len(collection))
		copy(result, collection)
		return result
	}
	result := make([]T, n)
	copy(result, collection[len(collection)-n:])
	return result
}

// Drop returns a new slice with the first n elements removed.
func Drop[T any](collection []T, n int) []T {
	if n <= 0 {
		result := make([]T, len(collection))
		copy(result, collection)
		return result
	}
	if n >= len(collection) {
		return []T{}
	}
	result := make([]T, len(collection)-n)
	copy(result, collection[n:])
	return result
}

// DropRight returns a new slice with the last n elements removed.
func DropRight[T any](collection []T, n int) []T {
	if n <= 0 {
		result := make([]T, len(collection))
		copy(result, collection)
		return result
	}
	if n >= len(collection) {
		return []T{}
	}
	result := make([]T, len(collection)-n)
	copy(result, collection[:len(collection)-n])
	return result
}

// ─── Grouping & Partitioning ──────────────────────────────────────────────────

// GroupBy groups elements by the key returned by iteratee.
//
//	groups := lo.GroupBy(words, func(w string) int { return len(w) })
func GroupBy[T any, U comparable](collection []T, iteratee func(item T) U) map[U][]T {
	result := make(map[U][]T)
	for _, v := range collection {
		key := iteratee(v)
		result[key] = append(result[key], v)
	}
	return result
}

// KeyBy builds a map keyed by iteratee. Duplicate keys overwrite earlier values.
//
//	byID := lo.KeyBy(users, func(u User) int { return u.ID })
func KeyBy[T any, K comparable](collection []T, iteratee func(item T) K) map[K]T {
	result := make(map[K]T, len(collection))
	for _, v := range collection {
		result[iteratee(v)] = v
	}
	return result
}

// Associate builds a map by applying transform to each element to produce key-value pairs.
//
//	m := lo.Associate(users, func(u User) (string, int) { return u.Name, u.Age })
func Associate[T any, K comparable, V any](collection []T, transform func(item T) (K, V)) map[K]V {
	result := make(map[K]V, len(collection))
	for _, v := range collection {
		k, val := transform(v)
		result[k] = val
	}
	return result
}

// Partition splits collection into two slices: elements satisfying predicate (left)
// and those not satisfying it (right).
//
//	pass, fail := lo.Partition(scores, func(s int) bool { return s >= 60 })
func Partition[T any](collection []T, predicate func(item T) bool) ([]T, []T) {
	yes := make([]T, 0, len(collection)/2)
	no := make([]T, 0, len(collection)/2)
	for _, v := range collection {
		if predicate(v) {
			yes = append(yes, v)
		} else {
			no = append(no, v)
		}
	}
	return yes, no
}

// ─── Set-like operations ──────────────────────────────────────────────────────

// Uniq returns a new slice with duplicate elements removed, preserving first-occurrence order.
func Uniq[T comparable](collection []T) []T {
	seen := make(map[T]struct{}, len(collection))
	result := make([]T, 0, len(collection))
	for _, v := range collection {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// UniqBy deduplicates using a key function; elements are kept on first occurrence
// of a given key value.
//
//	lo.UniqBy(words, strings.ToLower) // ["Hello", "world"]
func UniqBy[T any, U comparable](collection []T, iteratee func(item T) U) []T {
	seen := make(map[U]struct{}, len(collection))
	result := make([]T, 0, len(collection))
	for _, v := range collection {
		key := iteratee(v)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Intersect returns elements that appear in both list1 and list2 (distinct values).
func Intersect[T comparable](list1, list2 []T) []T {
	set2 := make(map[T]struct{}, len(list2))
	for _, v := range list2 {
		set2[v] = struct{}{}
	}
	added := make(map[T]struct{})
	result := make([]T, 0)
	for _, v := range list1 {
		if _, inSet2 := set2[v]; inSet2 {
			if _, alreadyAdded := added[v]; !alreadyAdded {
				result = append(result, v)
				added[v] = struct{}{}
			}
		}
	}
	return result
}

// Difference returns two slices: elements only in list1 and elements only in list2.
func Difference[T comparable](list1, list2 []T) (onlyIn1, onlyIn2 []T) {
	set1 := make(map[T]struct{}, len(list1))
	for _, v := range list1 {
		set1[v] = struct{}{}
	}
	set2 := make(map[T]struct{}, len(list2))
	for _, v := range list2 {
		set2[v] = struct{}{}
	}
	for _, v := range list1 {
		if _, ok := set2[v]; !ok {
			onlyIn1 = append(onlyIn1, v)
		}
	}
	for _, v := range list2 {
		if _, ok := set1[v]; !ok {
			onlyIn2 = append(onlyIn2, v)
		}
	}
	return
}

// Union merges any number of slices into one, removing duplicate values.
// Order of first occurrence is preserved.
func Union[T comparable](lists ...[]T) []T {
	seen := make(map[T]struct{})
	result := make([]T, 0)
	for _, list := range lists {
		for _, v := range list {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				result = append(result, v)
			}
		}
	}
	return result
}

// Without returns a new slice with all occurrences of exclude values removed.
//
//	lo.Without([]int{1, 2, 3, 4}, 2, 4) // [1, 3]
func Without[T comparable](collection []T, exclude ...T) []T {
	excl := make(map[T]struct{}, len(exclude))
	for _, v := range exclude {
		excl[v] = struct{}{}
	}
	result := make([]T, 0, len(collection))
	for _, v := range collection {
		if _, ok := excl[v]; !ok {
			result = append(result, v)
		}
	}
	return result
}

// ─── Shape operations ─────────────────────────────────────────────────────────

// Flatten merges a slice of slices into a single flat slice.
//
//	lo.Flatten([][]int{{1, 2}, {3, 4}}) // [1, 2, 3, 4]
func Flatten[T any](collection [][]T) []T {
	total := 0
	for _, v := range collection {
		total += len(v)
	}
	result := make([]T, 0, total)
	for _, v := range collection {
		result = append(result, v...)
	}
	return result
}

// Chunk splits collection into sub-slices of at most size elements.
// The last chunk may be smaller.
//
//	lo.Chunk([]int{1, 2, 3, 4, 5}, 2) // [[1,2],[3,4],[5]]
func Chunk[T any](collection []T, size int) [][]T {
	if size <= 0 {
		panic("lo: Chunk size must be positive")
	}
	result := make([][]T, 0, (len(collection)+size-1)/size)
	for i := 0; i < len(collection); i += size {
		end := i + size
		if end > len(collection) {
			end = len(collection)
		}
		chunk := make([]T, end-i)
		copy(chunk, collection[i:end])
		result = append(result, chunk)
	}
	return result
}

// Reverse returns a new slice with elements in reversed order.
func Reverse[T any](collection []T) []T {
	n := len(collection)
	result := make([]T, n)
	for i, v := range collection {
		result[n-1-i] = v
	}
	return result
}

// Compact returns a new slice with all zero-value elements removed.
//
//	lo.Compact([]string{"a", "", "b", ""}) // ["a", "b"]
func Compact[T comparable](collection []T) []T {
	var zero T
	return Filter(collection, func(v T, _ int) bool { return v != zero })
}

// Repeat returns a slice containing initial repeated count times.
//
//	lo.Repeat(3, "x") // ["x", "x", "x"]
func Repeat[T any](count int, initial T) []T {
	result := make([]T, count)
	for i := range result {
		result[i] = initial
	}
	return result
}

// Fill overwrites every element of collection with initial in place and returns it.
func Fill[T any](collection []T, initial T) []T {
	for i := range collection {
		collection[i] = initial
	}
	return collection
}

// Shuffle returns a new slice with elements in random order.
// Uses the default global random source (automatically seeded since Go 1.20).
func Shuffle[T any](collection []T) []T {
	result := make([]T, len(collection))
	copy(result, collection)
	rand.Shuffle(len(result), func(i, j int) {
		result[i], result[j] = result[j], result[i]
	})
	return result
}

// ─── Zip / Unzip ─────────────────────────────────────────────────────────────

// Zip combines two slices element-by-element into Tuple2 pairs.
// Length equals min(len(a), len(b)).
//
//	lo.Zip([]string{"a","b"}, []int{1,2}) // [{A:"a" B:1},{A:"b" B:2}]
func Zip[A, B any](a []A, b []B) []Tuple2[A, B] {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	result := make([]Tuple2[A, B], n)
	for i := 0; i < n; i++ {
		result[i] = T2(a[i], b[i])
	}
	return result
}

// Unzip splits a slice of Tuple2 into two separate slices.
func Unzip[A, B any](collection []Tuple2[A, B]) ([]A, []B) {
	as := make([]A, len(collection))
	bs := make([]B, len(collection))
	for i, t := range collection {
		as[i], bs[i] = t.A, t.B
	}
	return as, bs
}
