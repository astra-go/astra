package lo

// ─── Tuple types ─────────────────────────────────────────────────────────────

// Tuple2 is a generic pair of two values.
//
//	pair := lo.T2("alice", 30)
//	name, age := pair.Unpack()
type Tuple2[A, B any] struct {
	A A
	B B
}

// Unpack returns both values.
func (t Tuple2[A, B]) Unpack() (A, B) { return t.A, t.B }

// T2 constructs a Tuple2.
func T2[A, B any](a A, b B) Tuple2[A, B] { return Tuple2[A, B]{A: a, B: b} }

// Tuple3 holds three values.
//
//	triple := lo.T3("alice", 30, true)
//	name, age, active := triple.Unpack()
type Tuple3[A, B, C any] struct {
	A A
	B B
	C C
}

// Unpack returns all three values.
func (t Tuple3[A, B, C]) Unpack() (A, B, C) { return t.A, t.B, t.C }

// T3 constructs a Tuple3.
func T3[A, B, C any](a A, b B, c C) Tuple3[A, B, C] { return Tuple3[A, B, C]{A: a, B: b, C: c} }

// Tuple4 holds four values.
type Tuple4[A, B, C, D any] struct {
	A A
	B B
	C C
	D D
}

// Unpack returns all four values.
func (t Tuple4[A, B, C, D]) Unpack() (A, B, C, D) { return t.A, t.B, t.C, t.D }

// T4 constructs a Tuple4.
func T4[A, B, C, D any](a A, b B, c C, d D) Tuple4[A, B, C, D] {
	return Tuple4[A, B, C, D]{A: a, B: b, C: c, D: d}
}

// ─── Entry ───────────────────────────────────────────────────────────────────

// Entry is a key-value pair, used by map helpers such as Entries and FromEntries.
type Entry[K comparable, V any] struct {
	Key   K
	Value V
}
