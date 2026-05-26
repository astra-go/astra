package i18n

// messagesEN contains the built-in English translations.
//
// Validation message format: %s = field name, second %s / %v = validator param.
var messagesEN = Messages{
	// ── HTTP status errors ─────────────────────────────────────────────────────
	"http.400": "Bad Request",
	"http.401": "Unauthorized",
	"http.403": "Forbidden",
	"http.404": "Not Found",
	"http.405": "Method Not Allowed",
	"http.409": "Conflict",
	"http.422": "Unprocessable Entity",
	"http.429": "Too Many Requests",
	"http.500": "Internal Server Error",
	"http.503": "Service Unavailable",

	// ── Field-level validation errors ─────────────────────────────────────────
	// %s[0] = field name, %s[1] = param (min/max value, allowed values, …)
	"validate.required":     "%s is required",
	"validate.min":          "%s must be at least %s characters",
	"validate.max":          "%s must be at most %s characters",
	"validate.len":          "%s must be exactly %s characters",
	"validate.gte":          "%s must be greater than or equal to %s",
	"validate.lte":          "%s must be less than or equal to %s",
	"validate.gt":           "%s must be greater than %s",
	"validate.lt":           "%s must be less than %s",
	"validate.email":        "%s must be a valid email address",
	"validate.url":          "%s must be a valid URL",
	"validate.numeric":      "%s must be a number",
	"validate.alpha":        "%s must contain only letters",
	"validate.alphanum":     "%s must contain only letters and numbers",
	"validate.oneof":        "%s must be one of: %s",
	"validate.unique":       "%s must contain unique values",
	"validate.mobile":       "%s must be a valid mobile number",
	"validate.password":     "%s must be at least 8 characters and include uppercase, lowercase, digit and special character",
	"validate.username":     "%s must be 3–32 characters and contain only letters, digits and underscores",
	"validate.no_html":      "%s must not contain HTML tags",
	"validate.not_blank":    "%s must not be blank",

	// ── Common business messages ───────────────────────────────────────────────
	"common.success":        "Success",
	"common.created":        "Created successfully",
	"common.updated":        "Updated successfully",
	"common.deleted":        "Deleted successfully",
	"common.not_found":      "Resource not found",
	"common.unauthorized":   "Please log in to continue",
	"common.forbidden":      "You do not have permission to perform this action",
	"common.invalid_params": "Invalid request parameters",
	"common.server_error":   "An internal error occurred. Please try again later.",
	"common.rate_limited":   "Too many requests. Please slow down.",
	"common.token_expired":  "Session expired. Please log in again.",
	"common.token_invalid":  "Invalid or missing authentication token",
	"common.bad_content":    "Unsupported content type",
	"common.upload_too_large": "File is too large",
	"common.maintenance":    "Service is under maintenance. Please try again later.",
}
