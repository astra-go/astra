package i18n

// messagesKO contains the built-in Korean translations.
// Registered under "ko" by NewDefault.
var messagesKO = Messages{
	// ── HTTP 상태 오류 ────────────────────────────────────────────────────────
	"http.400": "잘못된 요청입니다",
	"http.401": "인증이 필요합니다",
	"http.403": "접근이 거부되었습니다",
	"http.404": "리소스를 찾을 수 없습니다",
	"http.405": "허용되지 않는 메서드입니다",
	"http.409": "데이터 충돌이 발생했습니다. 다시 시도해 주세요",
	"http.422": "요청 데이터 형식이 올바르지 않습니다",
	"http.429": "요청이 너무 많습니다. 잠시 후 다시 시도해 주세요",
	"http.500": "서버 내부 오류가 발생했습니다",
	"http.503": "서비스를 일시적으로 사용할 수 없습니다",

	// ── 필드 수준 유효성 검사 오류 ────────────────────────────────────────────
	"validate.required":  "%s 은(는) 필수입니다",
	"validate.min":       "%s 은(는) 최소 %s 자 이상이어야 합니다",
	"validate.max":       "%s 은(는) 최대 %s 자 이하이어야 합니다",
	"validate.len":       "%s 은(는) 정확히 %s 자이어야 합니다",
	"validate.gte":       "%s 은(는) %s 이상이어야 합니다",
	"validate.lte":       "%s 은(는) %s 이하이어야 합니다",
	"validate.gt":        "%s 은(는) %s 보다 커야 합니다",
	"validate.lt":        "%s 은(는) %s 보다 작아야 합니다",
	"validate.email":     "%s 은(는) 유효한 이메일 주소여야 합니다",
	"validate.url":       "%s 은(는) 유효한 URL이어야 합니다",
	"validate.numeric":   "%s 은(는) 숫자여야 합니다",
	"validate.alpha":     "%s 은(는) 영문자만 포함해야 합니다",
	"validate.alphanum":  "%s 은(는) 영문자와 숫자만 포함해야 합니다",
	"validate.oneof":     "%s 은(는) 다음 중 하나여야 합니다: %s",
	"validate.unique":    "%s 에 중복 값이 있으면 안 됩니다",
	"validate.mobile":    "%s 은(는) 유효한 휴대폰 번호여야 합니다",
	"validate.password":  "%s 은(는) 8자 이상이며 대문자, 소문자, 숫자, 특수문자를 포함해야 합니다",
	"validate.username":  "%s 은(는) 3~32자이며 영문자, 숫자, 밑줄만 사용할 수 있습니다",
	"validate.no_html":   "%s 에 HTML 태그를 포함할 수 없습니다",
	"validate.not_blank": "%s 은(는) 공백일 수 없습니다",

	// ── 공통 비즈니스 메시지 ──────────────────────────────────────────────────
	"common.success":          "성공했습니다",
	"common.created":          "생성되었습니다",
	"common.updated":          "수정되었습니다",
	"common.deleted":          "삭제되었습니다",
	"common.not_found":        "리소스를 찾을 수 없습니다",
	"common.unauthorized":     "계속하려면 로그인하세요",
	"common.forbidden":        "이 작업을 수행할 권한이 없습니다",
	"common.invalid_params":   "잘못된 요청 파라미터입니다",
	"common.server_error":     "내부 오류가 발생했습니다. 나중에 다시 시도해 주세요.",
	"common.rate_limited":     "요청이 너무 많습니다. 잠시 후 다시 시도해 주세요.",
	"common.token_expired":    "세션이 만료되었습니다. 다시 로그인하세요.",
	"common.token_invalid":    "인증 토큰이 유효하지 않거나 없습니다",
	"common.bad_content":      "지원되지 않는 콘텐츠 유형입니다",
	"common.upload_too_large": "파일이 너무 큽니다",
	"common.maintenance":      "서비스가 점검 중입니다. 나중에 다시 시도해 주세요.",
}
