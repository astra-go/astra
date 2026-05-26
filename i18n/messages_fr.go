package i18n

// messagesFR contains the built-in French translations.
// Registered under "fr" by NewDefault.
var messagesFR = Messages{
	// ── Erreurs de statut HTTP ────────────────────────────────────────────────
	"http.400": "Mauvaise requête",
	"http.401": "Non autorisé",
	"http.403": "Accès interdit",
	"http.404": "Ressource introuvable",
	"http.405": "Méthode non autorisée",
	"http.409": "Conflit de données, veuillez réessayer",
	"http.422": "Format de données invalide",
	"http.429": "Trop de requêtes, veuillez réessayer plus tard",
	"http.500": "Erreur interne du serveur",
	"http.503": "Service temporairement indisponible",

	// ── Erreurs de validation par champ ──────────────────────────────────────
	"validate.required":  "%s est obligatoire",
	"validate.min":       "%s doit contenir au moins %s caractères",
	"validate.max":       "%s doit contenir au maximum %s caractères",
	"validate.len":       "%s doit contenir exactement %s caractères",
	"validate.gte":       "%s doit être supérieur ou égal à %s",
	"validate.lte":       "%s doit être inférieur ou égal à %s",
	"validate.gt":        "%s doit être supérieur à %s",
	"validate.lt":        "%s doit être inférieur à %s",
	"validate.email":     "%s doit être une adresse e-mail valide",
	"validate.url":       "%s doit être une URL valide",
	"validate.numeric":   "%s doit être un nombre",
	"validate.alpha":     "%s ne doit contenir que des lettres",
	"validate.alphanum":  "%s ne doit contenir que des lettres et des chiffres",
	"validate.oneof":     "%s doit être l'une des valeurs suivantes : %s",
	"validate.unique":    "%s ne doit pas contenir de valeurs en double",
	"validate.mobile":    "%s doit être un numéro de mobile valide",
	"validate.password":  "%s doit comporter au moins 8 caractères et inclure une majuscule, une minuscule, un chiffre et un caractère spécial",
	"validate.username":  "%s doit contenir entre 3 et 32 caractères, uniquement des lettres, chiffres et underscores",
	"validate.no_html":   "%s ne doit pas contenir de balises HTML",
	"validate.not_blank": "%s ne doit pas être vide",

	// ── Messages métier communs ───────────────────────────────────────────────
	"common.success":          "Succès",
	"common.created":          "Créé avec succès",
	"common.updated":          "Mis à jour avec succès",
	"common.deleted":          "Supprimé avec succès",
	"common.not_found":        "Ressource introuvable",
	"common.unauthorized":     "Veuillez vous connecter pour continuer",
	"common.forbidden":        "Vous n'avez pas la permission d'effectuer cette action",
	"common.invalid_params":   "Paramètres de requête invalides",
	"common.server_error":     "Une erreur interne s'est produite. Veuillez réessayer plus tard.",
	"common.rate_limited":     "Trop de requêtes. Veuillez ralentir.",
	"common.token_expired":    "Session expirée. Veuillez vous reconnecter.",
	"common.token_invalid":    "Jeton d'authentification invalide ou manquant",
	"common.bad_content":      "Type de contenu non supporté",
	"common.upload_too_large": "Le fichier est trop volumineux",
	"common.maintenance":      "Le service est en maintenance. Veuillez réessayer plus tard.",
}
