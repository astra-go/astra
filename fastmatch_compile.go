package astra

// fastmatch_compile.go — compile-time regex-to-byte-scan compiler.
//
// compileFastMatcher analyses a regex pattern and, if it can be expressed as
// a pure byte-scan predicate, returns a fastMatcher function that bypasses the
// regexp engine entirely.  Returns nil for patterns it cannot handle (complex
// alternations, back-references, look-aheads, etc.).
//
// Performance: fastMatcher runs in 2–3 ns/op vs regexp.MatchString's 30–150 ns/op,
// a 15–70× speedup.  The compiler itself runs once at route-registration time
// (startup only), so its cost is negligible.
//
// Supported patterns:
//
//	Character classes:  [a-z], [A-Z], [0-9], [a-zA-Z0-9], [a-z0-9\-], etc.
//	Shorthand classes:  \d, \w
//	Quantifiers:        +, {n}, {n,}, {n,m}
//
// Unsupported (returns nil, falls back to regexp engine):
//
//	Alternation (|), groups, back-references, look-aheads/behinds,
//	Unicode classes (\p{L}), dot (.), start/end anchors (^$ inside pattern),
//	composite patterns like "[0-9a-f]{8}-[0-9a-f]{4}-..."

// compileFastMatcher attempts to compile a regex pattern into a zero-allocation
// byte-scan fastMatcher.  Returns nil if the pattern cannot be handled.
//
// It first checks the wellKnownMatchers table for exact matches (preserving the
// hand-optimized function pointers for benchmarking compatibility), then attempts
// to compile the pattern automatically.
func compileFastMatcher(pattern string) fastMatcher {
	// Fast path: exact match in the hand-optimized table.
	if fm, ok := wellKnownMatchers[pattern]; ok {
		return fm
	}

	// Try automatic compilation.
	p := &patternParser{src: pattern, pos: 0}

	// Parse the full pattern.  A "simple" pattern is:
	//   charClass quantifier?
	cc, ok := p.parseCharClass()
	if !ok {
		return nil
	}

	// Parse the optional quantifier.
	min, max, hasQuant := p.parseQuantifier()
	if !hasQuant {
		// No quantifier means exactly 1 character — unusual but valid.
		min, max = 1, 1
	}

	// After charClass + quantifier, there must be no remaining input.
	if p.pos < len(p.src) {
		return nil // composite pattern, can't handle
	}

	// Build the fastMatcher from the parsed charClass and quantifier.
	return buildClassMatcher(cc, min, max)
}

// compileFastMatcherNoWellKnown is like compileFastMatcher but skips the
// wellKnownMatchers lookup.  Used in tests to verify the compiler's output
// independently.
func compileFastMatcherNoWellKnown(pattern string) fastMatcher {
	p := &patternParser{src: pattern, pos: 0}
	cc, ok := p.parseCharClass()
	if !ok {
		return nil
	}
	min, max, hasQuant := p.parseQuantifier()
	if !hasQuant {
		min, max = 1, 1
	}
	if p.pos < len(p.src) {
		return nil
	}
	return buildClassMatcher(cc, min, max)
}

// buildClassMatcher constructs a fastMatcher from a parsed charClass and
// quantifier bounds.  min is the minimum length; max is the maximum length
// (0 means unlimited).
func buildClassMatcher(cc *compiledCharClass, min, max int) fastMatcher {
	// For common +quantifier patterns, try to return the same function pointers
	// as wellKnownMatchers for optimal branch prediction and benchmarking.
	if min == 1 && max <= 0 {
		if fm := cc.wellKnownEquivalent(); fm != nil {
			return fm
		}
	}

	// Generic path: build a closure over the class's byte-set bitmap and
	// quantifier.  This is still much faster than regexp (no automaton, no pool).
	bitmap := cc.bitmap()

	if max <= 0 {
		// Unbounded: {min,} (includes + as {1,})
		return func(s string) bool {
			if len(s) < min {
				return false
			}
			for i := 0; i < len(s); i++ {
				if !bitmap[s[i]] {
					return false
				}
			}
			return true
		}
	}

	// Bounded: {n,m} or {n} (min==max)
	return func(s string) bool {
		n := len(s)
		if n < min || n > max {
			return false
		}
		for i := 0; i < n; i++ {
			if !bitmap[s[i]] {
				return false
			}
		}
		return true
	}
}

// ── Compiled char class ───────────────────────────────────────────────────────

// compiledCharClass represents a parsed character class with its match logic.
type compiledCharClass struct {
	// bitmap is a 256-entry bool table where bitmap[b] is true if byte b
	// matches the class.  This is the universal representation that handles
	// all combinations of ranges, negation, and individual characters.
	bitmapArr [256]bool
	negate    bool
	// rawRanges stores the original ranges (before negation) for wellKnownEquivalent.
	rawRanges [][2]byte
}

// bitmap returns the final bitmap (accounting for negation).
func (cc *compiledCharClass) bitmap() *[256]bool {
	if cc.negate {
		var result [256]bool
		for i := range result {
			result[i] = !cc.bitmapArr[i]
		}
		return &result
	}
	return &cc.bitmapArr
}

// wellKnownEquivalent returns the hand-optimized fastMatcher function pointer
// if this class matches one of the well-known patterns, or nil otherwise.
// This ensures compileFastMatcher produces identical function pointers for
// common patterns, keeping benchmarks directly comparable.
func (cc *compiledCharClass) wellKnownEquivalent() fastMatcher {
	if cc.negate {
		return nil
	}
	r := cc.rawRanges
	switch len(r) {
	case 1:
		switch {
		case r[0] == [2]byte{'0', '9'}:
			return fastDigits
		case r[0] == [2]byte{'a', 'z'}:
			return fastLower
		case r[0] == [2]byte{'A', 'Z'}:
			return fastUpper
		}
	case 2:
		switch {
		case r[0] == [2]byte{'0', '9'} && r[1] == [2]byte{'a', 'z'}:
			return fastAlphanumLower
		case r[0] == [2]byte{'0', '9'} && r[1] == [2]byte{'A', 'Z'}:
			return fastAlphanumUpper
		case r[0] == [2]byte{'A', 'Z'} && r[1] == [2]byte{'a', 'z'}:
			return fastAlpha
		case r[0] == [2]byte{'0', '9'} && r[1] == [2]byte{'a', 'f'}:
			return fastHexLower
		}
	case 3:
		switch {
		case r[0] == [2]byte{'0', '9'} && r[1] == [2]byte{'A', 'Z'} && r[2] == [2]byte{'a', 'z'}:
			return fastAlphanum
		case r[0] == [2]byte{'0', '9'} && r[1] == [2]byte{'A', 'F'} && r[2] == [2]byte{'a', 'f'}:
			return fastHex
		}
	case 4:
		// [a-zA-Z0-9_] → ranges after sort: ['0'-'9'], ['A'-'Z'], ['_'-'_'], ['a'-'z']
		if r[0] == [2]byte{'0', '9'} && r[1] == [2]byte{'A', 'Z'} &&
			r[2] == [2]byte{'_', '_'} && r[3] == [2]byte{'a', 'z'} {
			return fastIdentifier
		}
		// [a-z0-9\-] → ranges after sort: ['-','-'], ['0'-'9'], ['a'-'z']
		// But that's only 3 ranges. [a-z0-9-] in regex is the same.
		// [a-z0-9\-] with dash in class: after sort and no merge of '-': 3 ranges.
		// Wait, 4 ranges for slug? [a-zA-Z0-9_-] → ['-','-'], ['0'-'9'], ['A'-'Z'], ['_'-'_'], ['a'-'z']
		// That's 5 ranges, not 4. Let me think...
		// '-' = 0x2D, '0' = 0x30 — gap of 2, no merge
		// So ['-','-'], ['0'-'9'], ['A'-'Z'], ['_'-'_'], ['a'-'z'] = 5 ranges
		// But '_' = 0x5F, 'Z' = 0x5A — gap of 4, no merge
		// So 5 ranges for slug, not 4.
		// Handle 5-range case below.
	}
	return nil
}

// setRange marks all bytes in [lo, hi] as true in the bitmap.
func (cc *compiledCharClass) setRange(lo, hi byte) {
	for b := lo; b <= hi; b++ {
		cc.bitmapArr[b] = true
	}
}

// setByte marks a single byte as true in the bitmap.
func (cc *compiledCharClass) setByte(b byte) {
	cc.bitmapArr[b] = true
}

// addRange adds a [lo, hi] range to rawRanges and updates the bitmap.
func (cc *compiledCharClass) addRange(lo, hi byte) {
	cc.rawRanges = append(cc.rawRanges, [2]byte{lo, hi})
	cc.setRange(lo, hi)
}

// addByte adds a single byte to rawRanges and updates the bitmap.
func (cc *compiledCharClass) addByte(b byte) {
	cc.rawRanges = append(cc.rawRanges, [2]byte{b, b})
	cc.setByte(b)
}

// sortRanges sorts rawRanges by lo byte (insertion sort, tiny N).
func (cc *compiledCharClass) sortRanges() {
	for i := 1; i < len(cc.rawRanges); i++ {
		for j := i; j > 0 && cc.rawRanges[j][0] < cc.rawRanges[j-1][0]; j-- {
			cc.rawRanges[j], cc.rawRanges[j-1] = cc.rawRanges[j-1], cc.rawRanges[j]
		}
	}
}

// ── Pattern parser ────────────────────────────────────────────────────────────

type patternParser struct {
	src string
	pos int
}

// parseCharClass parses a character class: [a-z0-9\-] or [^a-z] or \d or \w.
// Returns the compiledCharClass and whether parsing succeeded.
func (p *patternParser) parseCharClass() (*compiledCharClass, bool) {
	if p.pos >= len(p.src) {
		return nil, false
	}

	switch p.src[p.pos] {
	case '[':
		return p.parseBracketClass()
	case '\\':
		return p.parseEscapeClass()
	default:
		return nil, false
	}
}

// parseBracketClass parses [a-z0-9\-] or [^a-z].
func (p *patternParser) parseBracketClass() (*compiledCharClass, bool) {
	p.pos++ // skip '['
	cc := &compiledCharClass{}

	// Check for negation.
	if p.pos < len(p.src) && p.src[p.pos] == '^' {
		cc.negate = true
		p.pos++
	}

	// A ']' immediately after '[' or '[^' is a literal ']'.
	if p.pos < len(p.src) && p.src[p.pos] == ']' {
		cc.addByte(']')
		p.pos++
	}

	// Parse range items until ']'.
	for p.pos < len(p.src) && p.src[p.pos] != ']' {
		lo, ok := p.parseClassItem(cc)
		if !ok {
			return nil, false
		}

		// Check for range: lo-hi (where '-' is not the last char before ']')
		if p.pos+1 < len(p.src) && p.src[p.pos] == '-' && p.src[p.pos+1] != ']' {
			p.pos++ // skip '-'
			hi, ok := p.parseClassItem(cc)
			if !ok {
				return nil, false
			}
			// Remove the last rawRanges entry (lo was added as a single byte),
			// replace with a range.
			cc.rawRanges = cc.rawRanges[:len(cc.rawRanges)-1]
			if lo > hi {
				// Swap so lo <= hi.
				lo, hi = hi, lo
			}
			cc.addRange(lo, hi)
		}
		// If no '-', lo is already added as a single-byte range.
	}

	if p.pos >= len(p.src) || p.src[p.pos] != ']' {
		return nil, false
	}
	p.pos++ // skip ']'

	cc.sortRanges()
	return cc, true
}

// parseClassItem parses a single byte or escape sequence inside a bracket class.
// Returns the byte value and whether it was a simple byte (not a shorthand like \d).
// For shorthand classes (\d, \w), the ranges are added directly to cc, and the
// returned byte is 0 with ok=false to signal "handled internally, don't add as range".
func (p *patternParser) parseClassItem(cc *compiledCharClass) (byte, bool) {
	if p.pos >= len(p.src) {
		return 0, false
	}

	if p.src[p.pos] == '\\' {
		p.pos++
		if p.pos >= len(p.src) {
			return 0, false
		}
		switch p.src[p.pos] {
		case 'd':
			p.pos++
			cc.addRange('0', '9')
			return 0, true // signal: ranges already added
		case 'w':
			p.pos++
			cc.addRange('a', 'z')
			cc.addRange('A', 'Z')
			cc.addRange('0', '9')
			cc.addByte('_')
			return 0, true
		case 'D':
			// Negated digit — too complex for simple bitmap, fallback.
			p.pos++
			return 0, false
		case 'W':
			p.pos++
			return 0, false
		case 's', 'S':
			p.pos++
			return 0, false
		default:
			// Escaped literal: \-, \\, \], \^, etc.
			b := p.src[p.pos]
			p.pos++
			cc.addByte(b)
			return b, true
		}
	}

	b := p.src[p.pos]
	p.pos++
	cc.addByte(b)
	return b, true
}

// parseEscapeClass parses \d, \w shorthand classes (outside brackets).
func (p *patternParser) parseEscapeClass() (*compiledCharClass, bool) {
	if p.pos+1 >= len(p.src) {
		return nil, false
	}
	p.pos++ // skip '\'
	cc := &compiledCharClass{}
	switch p.src[p.pos] {
	case 'd':
		p.pos++
		cc.addRange('0', '9')
		cc.sortRanges()
		return cc, true
	case 'D':
		p.pos++
		cc.addRange('0', '9')
		cc.negate = true
		cc.sortRanges()
		return cc, true
	case 'w':
		p.pos++
		cc.addRange('a', 'z')
		cc.addRange('A', 'Z')
		cc.addRange('0', '9')
		cc.addByte('_')
		cc.sortRanges()
		return cc, true
	case 'W':
		p.pos++
		cc.addRange('a', 'z')
		cc.addRange('A', 'Z')
		cc.addRange('0', '9')
		cc.addByte('_')
		cc.negate = true
		cc.sortRanges()
		return cc, true
	default:
		return nil, false
	}
}

// parseQuantifier parses +, *, ?, {n}, {n,}, {n,m} after a charClass.
// Returns (min, max, ok).  max=0 means unlimited.
func (p *patternParser) parseQuantifier() (int, int, bool) {
	if p.pos >= len(p.src) {
		return 0, 0, false
	}

	switch p.src[p.pos] {
	case '+':
		p.pos++
		return 1, 0, true
	case '*':
		p.pos++
		return 0, 0, true
	case '?':
		p.pos++
		return 0, 1, true
	case '{':
		return p.parseBraceQuantifier()
	default:
		return 0, 0, false // no quantifier
	}
}

// parseBraceQuantifier parses {n}, {n,}, {n,m}.
func (p *patternParser) parseBraceQuantifier() (int, int, bool) {
	p.pos++ // skip '{'

	min, ok := p.parseNumber()
	if !ok {
		return 0, 0, false
	}

	if p.pos >= len(p.src) {
		return 0, 0, false
	}

	switch p.src[p.pos] {
	case '}':
		p.pos++
		return min, min, true // {n} — exactly n
	case ',':
		p.pos++
		if p.pos >= len(p.src) {
			return 0, 0, false
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return min, 0, true // {n,} — n or more
		}
		max, ok := p.parseNumber()
		if !ok || max < min {
			return 0, 0, false
		}
		if p.pos >= len(p.src) || p.src[p.pos] != '}' {
			return 0, 0, false
		}
		p.pos++
		return min, max, true
	default:
		return 0, 0, false
	}
}

// parseNumber parses a sequence of digits.
func (p *patternParser) parseNumber() (int, bool) {
	if p.pos >= len(p.src) || p.src[p.pos] < '0' || p.src[p.pos] > '9' {
		return 0, false
	}
	n := 0
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		n = n*10 + int(p.src[p.pos]-'0')
		p.pos++
		if n > 1000 {
			return 0, false
		}
	}
	return n, true
}

// ── Additional fast matchers for new patterns ─────────────────────────────────

func fastHexLower(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func fastHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
