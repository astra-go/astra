package i18n

// messagesDE contains the built-in German translations.
// Registered under "de" by NewDefault.
var messagesDE = Messages{
	// ── HTTP-Statusfehler ─────────────────────────────────────────────────────
	"http.400": "Ungültige Anfrage",
	"http.401": "Nicht autorisiert",
	"http.403": "Zugriff verweigert",
	"http.404": "Ressource nicht gefunden",
	"http.405": "Methode nicht erlaubt",
	"http.409": "Datenkonflikt, bitte versuchen Sie es erneut",
	"http.422": "Ungültiges Datenformat",
	"http.429": "Zu viele Anfragen, bitte versuchen Sie es später erneut",
	"http.500": "Interner Serverfehler",
	"http.503": "Dienst vorübergehend nicht verfügbar",

	// ── Feldvalidierungsfehler ────────────────────────────────────────────────
	"validate.required":  "%s ist erforderlich",
	"validate.min":       "%s muss mindestens %s Zeichen lang sein",
	"validate.max":       "%s darf höchstens %s Zeichen lang sein",
	"validate.len":       "%s muss genau %s Zeichen lang sein",
	"validate.gte":       "%s muss größer oder gleich %s sein",
	"validate.lte":       "%s muss kleiner oder gleich %s sein",
	"validate.gt":        "%s muss größer als %s sein",
	"validate.lt":        "%s muss kleiner als %s sein",
	"validate.email":     "%s muss eine gültige E-Mail-Adresse sein",
	"validate.url":       "%s muss eine gültige URL sein",
	"validate.numeric":   "%s muss eine Zahl sein",
	"validate.alpha":     "%s darf nur Buchstaben enthalten",
	"validate.alphanum":  "%s darf nur Buchstaben und Zahlen enthalten",
	"validate.oneof":     "%s muss eines der folgenden sein: %s",
	"validate.unique":    "%s darf keine doppelten Werte enthalten",
	"validate.mobile":    "%s muss eine gültige Mobilnummer sein",
	"validate.password":  "%s muss mindestens 8 Zeichen lang sein und Groß-, Kleinbuchstaben, Ziffern und Sonderzeichen enthalten",
	"validate.username":  "%s muss 3–32 Zeichen lang sein und darf nur Buchstaben, Ziffern und Unterstriche enthalten",
	"validate.no_html":   "%s darf keine HTML-Tags enthalten",
	"validate.not_blank": "%s darf nicht leer sein",

	// ── Allgemeine Geschäftsmeldungen ─────────────────────────────────────────
	"common.success":          "Erfolgreich",
	"common.created":          "Erfolgreich erstellt",
	"common.updated":          "Erfolgreich aktualisiert",
	"common.deleted":          "Erfolgreich gelöscht",
	"common.not_found":        "Ressource nicht gefunden",
	"common.unauthorized":     "Bitte melden Sie sich an, um fortzufahren",
	"common.forbidden":        "Sie haben keine Berechtigung, diese Aktion auszuführen",
	"common.invalid_params":   "Ungültige Anfrageparameter",
	"common.server_error":     "Ein interner Fehler ist aufgetreten. Bitte versuchen Sie es später erneut.",
	"common.rate_limited":     "Zu viele Anfragen. Bitte verlangsamen Sie sich.",
	"common.token_expired":    "Sitzung abgelaufen. Bitte melden Sie sich erneut an.",
	"common.token_invalid":    "Ungültiges oder fehlendes Authentifizierungstoken",
	"common.bad_content":      "Nicht unterstützter Inhaltstyp",
	"common.upload_too_large": "Datei ist zu groß",
	"common.maintenance":      "Der Dienst befindet sich im Wartungsmodus. Bitte versuchen Sie es später erneut.",
}
