package i18n

// messagesES contains the built-in Spanish translations.
// Registered under "es" by NewDefault.
var messagesES = Messages{
	// ── Errores de estado HTTP ────────────────────────────────────────────────
	"http.400": "Solicitud incorrecta",
	"http.401": "No autorizado",
	"http.403": "Acceso prohibido",
	"http.404": "Recurso no encontrado",
	"http.405": "Método no permitido",
	"http.409": "Conflicto de datos, por favor inténtelo de nuevo",
	"http.422": "Formato de datos inválido",
	"http.429": "Demasiadas solicitudes, por favor intente más tarde",
	"http.500": "Error interno del servidor",
	"http.503": "Servicio temporalmente no disponible",

	// ── Errores de validación por campo ──────────────────────────────────────
	"validate.required":  "%s es obligatorio",
	"validate.min":       "%s debe tener al menos %s caracteres",
	"validate.max":       "%s debe tener como máximo %s caracteres",
	"validate.len":       "%s debe tener exactamente %s caracteres",
	"validate.gte":       "%s debe ser mayor o igual a %s",
	"validate.lte":       "%s debe ser menor o igual a %s",
	"validate.gt":        "%s debe ser mayor que %s",
	"validate.lt":        "%s debe ser menor que %s",
	"validate.email":     "%s debe ser una dirección de correo electrónico válida",
	"validate.url":       "%s debe ser una URL válida",
	"validate.numeric":   "%s debe ser un número",
	"validate.alpha":     "%s debe contener solo letras",
	"validate.alphanum":  "%s debe contener solo letras y números",
	"validate.oneof":     "%s debe ser uno de: %s",
	"validate.unique":    "%s no debe contener valores duplicados",
	"validate.mobile":    "%s debe ser un número de móvil válido",
	"validate.password":  "%s debe tener al menos 8 caracteres e incluir mayúsculas, minúsculas, dígitos y caracteres especiales",
	"validate.username":  "%s debe tener entre 3 y 32 caracteres y contener solo letras, dígitos y guiones bajos",
	"validate.no_html":   "%s no debe contener etiquetas HTML",
	"validate.not_blank": "%s no debe estar en blanco",

	// ── Mensajes de negocio comunes ───────────────────────────────────────────
	"common.success":          "Éxito",
	"common.created":          "Creado correctamente",
	"common.updated":          "Actualizado correctamente",
	"common.deleted":          "Eliminado correctamente",
	"common.not_found":        "Recurso no encontrado",
	"common.unauthorized":     "Por favor inicie sesión para continuar",
	"common.forbidden":        "No tiene permiso para realizar esta acción",
	"common.invalid_params":   "Parámetros de solicitud inválidos",
	"common.server_error":     "Se produjo un error interno. Por favor inténtelo más tarde.",
	"common.rate_limited":     "Demasiadas solicitudes. Por favor espere un momento.",
	"common.token_expired":    "Sesión expirada. Por favor inicie sesión nuevamente.",
	"common.token_invalid":    "Token de autenticación inválido o ausente",
	"common.bad_content":      "Tipo de contenido no soportado",
	"common.upload_too_large": "El archivo es demasiado grande",
	"common.maintenance":      "El servicio está en mantenimiento. Por favor inténtelo más tarde.",
}
