package astra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ─── I18n: lightweight internationalization ───────────────────────────────────

// I18nManager provides JSON-file-driven message internationalization.
//
// Usage:
//
//	// Initialize on startup
//	manager := astra.NewI18nManager()
//	manager.LoadFromDir("./i18n") // loads en.json, zh.json, ja.json
//
//	// Localize an error message
//	msg := manager.Localize("zh", "error.usc.auth.1001")
//
//	// Or use the global singleton
//	astra.GetI18nManager().LoadFromDir("./i18n")
type I18nManager struct {
	mu       sync.RWMutex
	messages map[string]map[string]string // lang → key → message
	i18nDir  string                        // JSON file directory
}

// NewI18nManager creates a new I18nManager.
func NewI18nManager() *I18nManager {
	return &I18nManager{
		messages: map[string]map[string]string{
			"en": {},
			"zh": {},
			"ja": {},
		},
		i18nDir: "i18n",
	}
}

// Global I18nManager singleton.
var (
	globalI18n     *I18nManager
	globalI18nOnce sync.Once
)

// GetI18nManager returns the global I18nManager singleton.
// First call initializes with default language maps.
func GetI18nManager() *I18nManager {
	globalI18nOnce.Do(func() {
		globalI18n = NewI18nManager()
	})
	return globalI18n
}

// SetI18nDir sets the directory containing i18n JSON files.
func (m *I18nManager) SetI18nDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.i18nDir = dir
}

// LoadFromDir loads all JSON files from the given directory.
// File naming convention: <lang>.json (e.g. en.json, zh.json).
func (m *I18nManager) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("i18n: read dir %s: %w", dir, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if strings.ToLower(ext) != ".json" {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ext)
		filePath := filepath.Join(dir, entry.Name())

		if err := m.loadJSONFileLocked(lang, filePath); err != nil {
			// Log a warning but continue loading other files
			fmt.Fprintf(os.Stderr, "i18n: warning: failed to load %s: %v\n", filePath, err)
			continue
		}
	}
	return nil
}

// loadJSONFileLocked loads a single JSON file (must hold m.mu).
func (m *I18nManager) loadJSONFileLocked(lang, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var messages map[string]string
	if err := json.Unmarshal(data, &messages); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	if m.messages[lang] == nil {
		m.messages[lang] = messages
	} else {
		for k, v := range messages {
			m.messages[lang][k] = v
		}
	}
	return nil
}

// RegisterLang registers language messages directly (from code, not file).
func (m *I18nManager) RegisterLang(lang string, messages map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.messages[lang] == nil {
		m.messages[lang] = messages
	} else {
		for k, v := range messages {
			m.messages[lang][k] = v
		}
	}
}

// Localize returns the localized message for the given key and language.
// Falls back to English, then to the key itself if no translation is found.
func (m *I18nManager) Localize(lang, key string, args ...any) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Prefer requested language
	if msgs, ok := m.messages[lang]; ok {
		if tmpl, ok := msgs[key]; ok {
			if len(args) > 0 {
				return fmt.Sprintf(tmpl, args...)
			}
			return tmpl
		}
	}

	// Fallback: English
	if lang != "en" {
		if msgs, ok := m.messages["en"]; ok {
			if tmpl, ok := msgs[key]; ok {
				if len(args) > 0 {
					return fmt.Sprintf(tmpl, args...)
				}
				return tmpl
			}
		}
	}

	// Final fallback: return the key itself
	return key
}

// ─── I18n key convention ─────────────────────────────────────────────────────

// ErrorCodeToI18nKey converts a standard error code to an i18n lookup key.
//
//	"USC-AUTH-1001" → "error.usc.auth.1001"
//	"ORD-VAL-2001"  → "error.ord.val.2001"
func ErrorCodeToI18nKey(code string) string {
	parts := splitHyphen(code)
	if len(parts) >= 3 {
		svc := strings.ToLower(parts[0])
		cat := strings.ToLower(parts[1])
		num := parts[2]
		return fmt.Sprintf("error.%s.%s.%s", svc, cat, num)
	}
	if len(parts) == 2 {
		svc := strings.ToLower(parts[0])
		num := parts[1]
		return fmt.Sprintf("error.%s.%s", svc, num)
	}
	return "error.unknown"
}

// This file implements part of the enhanced error system documented in
// astra/errors.go.  The I18nManager is a standalone utility that can also
// be used for non-error localization needs.
