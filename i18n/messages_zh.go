package i18n

// messagesZH contains the built-in Simplified Chinese translations.
// Registered under both "zh" and "zh-CN" by NewDefault.
//
// 验证消息格式：%s[0] = 字段名，%s[1] = 校验参数（最小值/最大值/可选值等）
var messagesZH = Messages{
	// ── HTTP 状态错误 ─────────────────────────────────────────────────────────
	"http.400": "请求参数错误",
	"http.401": "未授权，请先登录",
	"http.403": "无权限访问",
	"http.404": "资源不存在",
	"http.405": "请求方法不允许",
	"http.409": "数据冲突，请刷新后重试",
	"http.422": "请求数据格式有误",
	"http.429": "请求过于频繁，请稍后再试",
	"http.500": "服务器内部错误，请稍后再试",
	"http.503": "服务暂时不可用，请稍后再试",

	// ── 字段级校验错误 ────────────────────────────────────────────────────────
	// %s[0] = 字段名，%s[1] = 参数（如最小长度、最大长度、可选值）
	"validate.required":     "%s 不能为空",
	"validate.min":          "%s 最少 %s 个字符",
	"validate.max":          "%s 最多 %s 个字符",
	"validate.len":          "%s 必须为 %s 个字符",
	"validate.gte":          "%s 必须大于或等于 %s",
	"validate.lte":          "%s 必须小于或等于 %s",
	"validate.gt":           "%s 必须大于 %s",
	"validate.lt":           "%s 必须小于 %s",
	"validate.email":        "%s 必须是有效的电子邮箱地址",
	"validate.url":          "%s 必须是有效的 URL",
	"validate.numeric":      "%s 必须是数字",
	"validate.alpha":        "%s 只能包含字母",
	"validate.alphanum":     "%s 只能包含字母和数字",
	"validate.oneof":        "%s 必须是以下选项之一：%s",
	"validate.unique":       "%s 中不能有重复值",
	"validate.mobile":       "%s 必须是有效的手机号码",
	"validate.password":     "%s 至少 8 个字符，需包含大写字母、小写字母、数字和特殊字符",
	"validate.username":     "%s 为 3～32 个字符，只能包含字母、数字和下划线",
	"validate.no_html":      "%s 不能包含 HTML 标签",
	"validate.not_blank":    "%s 不能为空白字符",

	// ── 通用业务提示 ──────────────────────────────────────────────────────────
	"common.success":          "操作成功",
	"common.created":          "创建成功",
	"common.updated":          "更新成功",
	"common.deleted":          "删除成功",
	"common.not_found":        "资源不存在",
	"common.unauthorized":     "请先登录后再操作",
	"common.forbidden":        "您没有权限执行此操作",
	"common.invalid_params":   "请求参数有误",
	"common.server_error":     "服务器内部错误，请稍后再试",
	"common.rate_limited":     "请求过于频繁，请稍后再试",
	"common.token_expired":    "登录已过期，请重新登录",
	"common.token_invalid":    "无效或缺少认证令牌",
	"common.bad_content":      "不支持的内容类型",
	"common.upload_too_large": "上传文件过大",
	"common.maintenance":      "系统正在维护中，请稍后再试",
}
