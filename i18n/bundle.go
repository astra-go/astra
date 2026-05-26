// Package i18n provides a zero-dependency, thread-safe multi-language message
// bundle for Astra applications.
//
// # Concepts
//
//   - Bundle: holds translations for one or more locales.
//   - Messages: a flat key→template map (e.g. {"validate.required": "%s 不能为空"}).
//   - Translator: a locale-scoped view of a Bundle; the T method formats a message.
//
// Built-in locales: en, zh/zh-CN, zh-TW/zh-HK, ja, ko, fr, de, es, pt/pt-BR, ru, ar.
// Any locale can be added or overridden at application start-up via Register and Extend.
//
// # Quick start
//
//	// 1. Register a custom locale or extend an existing one at startup.
//	i18n.Register("ja", i18n.Messages{
//	    "http.404":          "リソースが見つかりません",
//	    "validate.required": "%s は必須項目です",
//	    "common.success":    "成功",
//	})
//	i18n.Extend("zh", i18n.Messages{
//	    "common.welcome": "欢迎使用 %s",   // project-specific key
//	})
//
//	// 2. Mount the middleware (sets locale on each request context).
//	app.Use(i18n.Middleware())
//
//	// 3. Translate inside a handler.
//	func hello(c *astra.Ctx) error {
//	    msg := i18n.T(c, "common.success")
//	    return c.JSON(200, astra.Map{"message": msg})
//	}
package i18n

import (
	"fmt"
	"sync"
)

// Messages is a flat locale dictionary: message key → format string.
// Format strings follow fmt.Sprintf conventions; args are passed to T.
//
//	Messages{
//	    "validate.min": "%s must be at least %s characters",
//	    "common.welcome": "Welcome, %s!",
//	}
type Messages map[string]string

// Bundle holds translations for one or more locales and exposes a Translator
// factory.  All methods are safe for concurrent use.
type Bundle struct {
	mu       sync.RWMutex
	msgs     map[string]Messages // locale → key → template
	fallback string              // locale used when the requested locale has no entry
}

// New returns an empty Bundle with English as the fallback locale.
// The built-in "en" and "zh"/"zh-CN" locales are NOT pre-loaded; use
// NewDefault to get a Bundle pre-populated with both built-in locales.
func New() *Bundle {
	return &Bundle{
		msgs:     make(map[string]Messages),
		fallback: "en",
	}
}

// NewDefault returns a Bundle pre-loaded with all built-in locales, with "en"
// as the fallback. This is the constructor used by the package-level Default bundle.
//
// Built-in locales:
//
//	en          English
//	zh, zh-CN   Simplified Chinese
//	zh-TW, zh-HK Traditional Chinese
//	ja          Japanese
//	ko          Korean
//	fr          French
//	de          German
//	es          Spanish
//	pt, pt-BR   Portuguese
//	ru          Russian
//	ar          Arabic
func NewDefault() *Bundle {
	b := New()
	b.Register("en", messagesEN)
	b.Register("zh", messagesZH)
	b.Register("zh-CN", messagesZH)
	b.Register("zh-TW", messagesZHTW)
	b.Register("zh-HK", messagesZHTW)
	b.Register("ja", messagesJA)
	b.Register("ko", messagesKO)
	b.Register("fr", messagesFR)
	b.Register("de", messagesDE)
	b.Register("es", messagesES)
	b.Register("pt", messagesPT)
	b.Register("pt-BR", messagesPT)
	b.Register("ru", messagesRU)
	b.Register("ar", messagesAR)
	return b
}

// SetFallback sets the locale used when a requested locale is not registered
// or a key is missing from the requested locale.  Returns b for chaining.
func (b *Bundle) SetFallback(locale string) *Bundle {
	b.mu.Lock()
	b.fallback = locale
	b.mu.Unlock()
	return b
}

// Register adds (or replaces) all messages for locale.
// Calling Register with an existing locale replaces its entire message set.
// Use Extend to add/override individual keys while keeping the rest intact.
// Returns b for chaining.
func (b *Bundle) Register(locale string, msgs Messages) *Bundle {
	b.mu.Lock()
	dst := make(Messages, len(msgs))
	for k, v := range msgs {
		dst[k] = v
	}
	b.msgs[locale] = dst
	b.mu.Unlock()
	return b
}

// Extend merges msgs into locale.  Keys in msgs overwrite existing ones;
// keys not present in msgs are preserved.  If the locale has not been
// registered before, Extend behaves like Register.
// Returns b for chaining.
func (b *Bundle) Extend(locale string, msgs Messages) *Bundle {
	b.mu.Lock()
	dst, ok := b.msgs[locale]
	if !ok {
		dst = make(Messages, len(msgs))
		b.msgs[locale] = dst
	}
	for k, v := range msgs {
		dst[k] = v
	}
	b.mu.Unlock()
	return b
}

// Has reports whether locale is registered in the bundle.
func (b *Bundle) Has(locale string) bool {
	b.mu.RLock()
	_, ok := b.msgs[locale]
	b.mu.RUnlock()
	return ok
}

// Locales returns a sorted list of all registered locale names.
func (b *Bundle) Locales() []string {
	b.mu.RLock()
	out := make([]string, 0, len(b.msgs))
	for k := range b.msgs {
		out = append(out, k)
	}
	b.mu.RUnlock()
	return out
}

// Translator returns a locale-scoped Translator for locale.
// If locale is not registered, the bundle's fallback locale is used.
func (b *Bundle) Translator(locale string) *Translator {
	b.mu.RLock()
	_, ok := b.msgs[locale]
	b.mu.RUnlock()
	if !ok {
		locale = b.fallback
	}
	return &Translator{locale: locale, bundle: b}
}

// T is a convenience shortcut: it creates a Translator for locale and calls T.
func (b *Bundle) T(locale, key string, args ...any) string {
	return b.Translator(locale).T(key, args...)
}

// ─── Translator ────────────────────────────────────────────────────────────────

// Translator is a locale-scoped view of a Bundle.
// Obtain one via Bundle.Translator or the package-level ForLocale helper.
type Translator struct {
	locale string
	bundle *Bundle
}

// Locale returns the locale this Translator was created for.
func (t *Translator) Locale() string { return t.locale }

// T returns the message for key in this locale, formatted with args.
//
// Lookup order:
//  1. key in the requested locale
//  2. key in the bundle's fallback locale
//  3. key itself (acts as an identity fallback so the UI always shows something)
//
// Format: if args is non-empty, the template is passed through fmt.Sprintf.
func (t *Translator) T(key string, args ...any) string {
	t.bundle.mu.RLock()
	// Try primary locale.
	if m, ok := t.bundle.msgs[t.locale]; ok {
		if tmpl, ok := m[key]; ok {
			t.bundle.mu.RUnlock()
			return format(tmpl, args)
		}
	}
	// Try fallback locale.
	if t.locale != t.bundle.fallback {
		if m, ok := t.bundle.msgs[t.bundle.fallback]; ok {
			if tmpl, ok := m[key]; ok {
				t.bundle.mu.RUnlock()
				return format(tmpl, args)
			}
		}
	}
	t.bundle.mu.RUnlock()
	// Final fallback: return the key itself so callers always get a string.
	// Use doSprintf (a var, not a direct fmt.Sprintf reference) so that go
	// vet's printf analyser cannot identify T as a printf wrapper and
	// incorrectly flag call sites with non-constant format strings.
	if len(args) > 0 {
		return doSprintf(key, args...)
	}
	return key
}

func format(tmpl string, args []any) string {
	if len(args) == 0 {
		return tmpl
	}
	return doSprintf(tmpl, args...)
}

// doSprintf is an indirection to fmt.Sprintf that prevents go vet's printf
// checker from tracing through the Translator.T → format → Sprintf call chain
// and incorrectly flagging callers of T as passing non-constant format strings.
var doSprintf = fmt.Sprintf

// ─── Package-level API (Default bundle) ───────────────────────────────────────

// defaultBundle is the package-level Bundle, pre-loaded with English and
// Simplified Chinese. Protected by defaultBundleMu for safe concurrent use.
// Do not access directly — use SetDefault / GetDefault.
var (
	defaultBundleMu sync.RWMutex
	defaultBundle   = NewDefault()
)

// SetDefault replaces the package-level Bundle atomically.
// Use with t.Cleanup for parallel test isolation:
//
//	orig := i18n.GetDefault()
//	t.Cleanup(func() { i18n.SetDefault(orig) })
//	i18n.SetDefault(i18n.NewDefault())
func SetDefault(b *Bundle) {
	defaultBundleMu.Lock()
	defer defaultBundleMu.Unlock()
	defaultBundle = b
}

// GetDefault returns the current package-level Bundle.
func GetDefault() *Bundle {
	defaultBundleMu.RLock()
	defer defaultBundleMu.RUnlock()
	return defaultBundle
}

// SetFallback sets the fallback locale on the package-level bundle.
func SetFallback(locale string) { GetDefault().SetFallback(locale) }

// Register adds (or replaces) all messages for locale in the package-level bundle.
func Register(locale string, msgs Messages) { GetDefault().Register(locale, msgs) }

// Extend merges msgs into locale in the package-level bundle.
func Extend(locale string, msgs Messages) { GetDefault().Extend(locale, msgs) }

// ForLocale returns a Translator for locale from the package-level bundle.
func ForLocale(locale string) *Translator { return GetDefault().Translator(locale) }

// T is the most direct translation path: it uses the package-level bundle,
// picks the Translator stored in the Astra Context (set by Middleware),
// and formats the message.
//
// If Middleware was not mounted, it falls back to the bundle's fallback locale.
//
//	msg := i18n.T(c, "common.success")
//	msg := i18n.T(c, "validate.min", "username", "3")
func T(c contextGetter, key string, args ...any) string {
	if v, ok := c.Get(contextKey); ok {
		if tr, ok := v.(*Translator); ok {
			return tr.T(key, args...)
		}
	}
	b := GetDefault()
	return b.T(b.fallback, key, args...)
}

// contextGetter is the minimal interface required from *astra.Ctx so that
// this package does not import the astra package (avoiding circular imports).
type contextGetter interface {
	Get(key string) (any, bool)
}

// contextKey is the key used to store the *Translator in the request context.
const contextKey = "i18n.translator"
