package lo

import "fmt"

// ─── Ternary ─────────────────────────────────────────────────────────────────

// Ternary is a single-expression ternary operator.
// Both ifOutput and elseOutput are always evaluated (eager).
//
//	label := lo.Ternary(count > 1, "items", "item")
func Ternary[T any](condition bool, ifOutput, elseOutput T) T {
	if condition {
		return ifOutput
	}
	return elseOutput
}

// TernaryF is like Ternary but evaluates only the chosen branch (lazy).
//
//	result := lo.TernaryF(expensive, func() string { return compute() }, func() string { return cached })
func TernaryF[T any](condition bool, ifFunc, elseFunc func() T) T {
	if condition {
		return ifFunc()
	}
	return elseFunc()
}

// ─── If chain ─────────────────────────────────────────────────────────────────

// IfChain supports fluent multi-branch conditional expressions.
//
//	grade := lo.If(score >= 90, "A").
//	    ElseIf(score >= 80, "B").
//	    ElseIf(score >= 70, "C").
//	    Else("F")
type IfChain[T any] struct {
	result T
	done   bool
}

// If starts a new IfChain. If condition is true, value is returned when the chain ends.
func If[T any](condition bool, value T) *IfChain[T] {
	c := &IfChain[T]{}
	if condition {
		c.result = value
		c.done = true
	}
	return c
}

// IfF is like If but evaluates fn lazily, calling it only when condition is true.
func IfF[T any](condition bool, fn func() T) *IfChain[T] {
	c := &IfChain[T]{}
	if condition {
		c.result = fn()
		c.done = true
	}
	return c
}

// ElseIf adds a conditional branch.
func (c *IfChain[T]) ElseIf(condition bool, value T) *IfChain[T] {
	if !c.done && condition {
		c.result = value
		c.done = true
	}
	return c
}

// ElseIfF is like ElseIf but evaluates fn lazily.
func (c *IfChain[T]) ElseIfF(condition bool, fn func() T) *IfChain[T] {
	if !c.done && condition {
		c.result = fn()
		c.done = true
	}
	return c
}

// Else terminates the chain, returning the matched value or the supplied fallback.
func (c *IfChain[T]) Else(value T) T {
	if !c.done {
		return value
	}
	return c.result
}

// ElseF is like Else but evaluates fn lazily.
func (c *IfChain[T]) ElseF(fn func() T) T {
	if !c.done {
		return fn()
	}
	return c.result
}

// ─── Must helpers ─────────────────────────────────────────────────────────────

// Must panics if err is non-nil; otherwise returns val.
// Useful for wrapping calls that return (T, error) where errors are truly unexpected.
//
//	data := lo.Must(os.ReadFile("config.json"))
func Must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

// Must0 panics if err is non-nil. Use when there is no return value to unwrap.
//
//	lo.Must0(db.AutoMigrate(&User{}))
func Must0(err error) {
	if err != nil {
		panic(err)
	}
}

// Must2 panics if err is non-nil; otherwise returns both values.
func Must2[T1, T2 any](val1 T1, val2 T2, err error) (T1, T2) {
	if err != nil {
		panic(err)
	}
	return val1, val2
}

// ─── Try helpers ──────────────────────────────────────────────────────────────

// Try calls callback and returns true on success, false if it returns a non-nil error
// or panics. The panic is swallowed.
//
//	ok := lo.Try(func() error { return riskyOp() })
func Try(callback func() error) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	return callback() == nil
}

// TryCatch calls callback; on error or panic, catch is called with the error/panic value.
//
//	lo.TryCatch(
//	    func() error { return riskyOp() },
//	    func(err any) { log.Println("caught:", err) },
//	)
func TryCatch(callback func() error, catch func(err any)) {
	defer func() {
		if r := recover(); r != nil {
			catch(r)
		}
	}()
	if err := callback(); err != nil {
		catch(err)
	}
}

// TryWithErrorValue calls callback and returns (result, err, ok).
// If callback panics, the panic value is wrapped as an error and ok is false.
//
//	val, err, ok := lo.TryWithErrorValue(func() (int, error) { return parse(s) })
func TryWithErrorValue[T any](callback func() (T, error)) (result T, err error, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			switch v := r.(type) {
			case error:
				err = v
			default:
				err = fmt.Errorf("lo: recovered panic: %v", v)
			}
			ok = false
		}
	}()
	result, err = callback()
	ok = err == nil
	return
}
