package lo

// Min returns the minimum value in collection using < comparison.
// Panics on an empty slice.
//
//	lo.Min([]int{3, 1, 4, 1, 5}) // 1
func Min[T Ordered](collection []T) T {
	if len(collection) == 0 {
		panic("lo: Min called on empty slice")
	}
	m := collection[0]
	for _, v := range collection[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

// MinBy returns the element for which comparison(candidate, current) returns true
// (i.e., comparison acts as "candidate is smaller than current").
// Panics on an empty slice.
//
//	lo.MinBy(users, func(a, b User) bool { return a.Age < b.Age })
func MinBy[T any](collection []T, comparison func(a, b T) bool) T {
	if len(collection) == 0 {
		panic("lo: MinBy called on empty slice")
	}
	m := collection[0]
	for _, v := range collection[1:] {
		if comparison(v, m) {
			m = v
		}
	}
	return m
}

// Max returns the maximum value in collection using > comparison.
// Panics on an empty slice.
//
//	lo.Max([]float64{1.2, 3.7, 2.5}) // 3.7
func Max[T Ordered](collection []T) T {
	if len(collection) == 0 {
		panic("lo: Max called on empty slice")
	}
	m := collection[0]
	for _, v := range collection[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// MaxBy returns the element for which comparison(candidate, current) returns true
// (i.e., comparison acts as "candidate is greater than current").
// Panics on an empty slice.
func MaxBy[T any](collection []T, comparison func(a, b T) bool) T {
	if len(collection) == 0 {
		panic("lo: MaxBy called on empty slice")
	}
	m := collection[0]
	for _, v := range collection[1:] {
		if comparison(v, m) {
			m = v
		}
	}
	return m
}

// Sum returns the sum of all elements (zero for empty slices).
//
//	lo.Sum([]int{1, 2, 3, 4}) // 10
func Sum[T Number](collection []T) T {
	var total T
	for _, v := range collection {
		total += v
	}
	return total
}

// SumBy sums the numeric result of applying iteratee to each element.
//
//	lo.SumBy(orders, func(o Order) float64 { return o.Total })
func SumBy[T any, R Number](collection []T, iteratee func(item T) R) R {
	var total R
	for _, v := range collection {
		total += iteratee(v)
	}
	return total
}

// Clamp restricts value to the closed interval [min, max].
//
//	lo.Clamp(150, 0, 100) // 100
//	lo.Clamp(-5, 0, 100)  // 0
//	lo.Clamp(42, 0, 100)  // 42
func Clamp[T Ordered](value, min, max T) T {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
