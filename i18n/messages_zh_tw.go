package i18n

// messagesZHTW contains the built-in Traditional Chinese translations.
// Registered under "zh-TW" and "zh-HK" by NewDefault.
var messagesZHTW = Messages{
	// ── HTTP 狀態錯誤 ─────────────────────────────────────────────────────────
	"http.400": "請求參數錯誤",
	"http.401": "未授權，請先登入",
	"http.403": "無權限存取",
	"http.404": "資源不存在",
	"http.405": "請求方法不允許",
	"http.409": "資料衝突，請重新整理後再試",
	"http.422": "請求資料格式有誤",
	"http.429": "請求過於頻繁，請稍後再試",
	"http.500": "伺服器內部錯誤，請稍後再試",
	"http.503": "服務暫時無法使用，請稍後再試",

	// ── 欄位級驗證錯誤 ────────────────────────────────────────────────────────
	"validate.required":  "%s 不能為空",
	"validate.min":       "%s 最少 %s 個字元",
	"validate.max":       "%s 最多 %s 個字元",
	"validate.len":       "%s 必須為 %s 個字元",
	"validate.gte":       "%s 必須大於或等於 %s",
	"validate.lte":       "%s 必須小於或等於 %s",
	"validate.gt":        "%s 必須大於 %s",
	"validate.lt":        "%s 必須小於 %s",
	"validate.email":     "%s 必須是有效的電子郵件地址",
	"validate.url":       "%s 必須是有效的 URL",
	"validate.numeric":   "%s 必須是數字",
	"validate.alpha":     "%s 只能包含字母",
	"validate.alphanum":  "%s 只能包含字母和數字",
	"validate.oneof":     "%s 必須是以下選項之一：%s",
	"validate.unique":    "%s 中不能有重複值",
	"validate.mobile":    "%s 必須是有效的手機號碼",
	"validate.password":  "%s 至少 8 個字元，需包含大寫字母、小寫字母、數字和特殊字元",
	"validate.username":  "%s 為 3～32 個字元，只能包含字母、數字和底線",
	"validate.no_html":   "%s 不能包含 HTML 標籤",
	"validate.not_blank": "%s 不能為空白字元",

	// ── 通用業務提示 ──────────────────────────────────────────────────────────
	"common.success":          "操作成功",
	"common.created":          "建立成功",
	"common.updated":          "更新成功",
	"common.deleted":          "刪除成功",
	"common.not_found":        "資源不存在",
	"common.unauthorized":     "請先登入後再操作",
	"common.forbidden":        "您沒有權限執行此操作",
	"common.invalid_params":   "請求參數有誤",
	"common.server_error":     "伺服器內部錯誤，請稍後再試",
	"common.rate_limited":     "請求過於頻繁，請稍後再試",
	"common.token_expired":    "登入已過期，請重新登入",
	"common.token_invalid":    "無效或缺少認證令牌",
	"common.bad_content":      "不支援的內容類型",
	"common.upload_too_large": "上傳檔案過大",
	"common.maintenance":      "系統正在維護中，請稍後再試",
}
