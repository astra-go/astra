package i18n

// messagesJA contains the built-in Japanese translations.
// Registered under "ja" by NewDefault.
var messagesJA = Messages{
	// ── HTTP ステータスエラー ──────────────────────────────────────────────────
	"http.400": "リクエストが不正です",
	"http.401": "認証が必要です",
	"http.403": "アクセスが拒否されました",
	"http.404": "リソースが見つかりません",
	"http.405": "このメソッドは許可されていません",
	"http.409": "データが競合しています。再試行してください",
	"http.422": "リクエストデータの形式が正しくありません",
	"http.429": "リクエストが多すぎます。しばらくお待ちください",
	"http.500": "サーバー内部エラーが発生しました",
	"http.503": "サービスは一時的に利用できません",

	// ── フィールドレベルの検証エラー ─────────────────────────────────────────
	"validate.required":  "%s は必須です",
	"validate.min":       "%s は %s 文字以上である必要があります",
	"validate.max":       "%s は %s 文字以下である必要があります",
	"validate.len":       "%s は %s 文字である必要があります",
	"validate.gte":       "%s は %s 以上である必要があります",
	"validate.lte":       "%s は %s 以下である必要があります",
	"validate.gt":        "%s は %s より大きい必要があります",
	"validate.lt":        "%s は %s より小さい必要があります",
	"validate.email":     "%s は有効なメールアドレスである必要があります",
	"validate.url":       "%s は有効な URL である必要があります",
	"validate.numeric":   "%s は数値である必要があります",
	"validate.alpha":     "%s は英字のみを含む必要があります",
	"validate.alphanum":  "%s は英数字のみを含む必要があります",
	"validate.oneof":     "%s は次のいずれかである必要があります：%s",
	"validate.unique":    "%s に重複する値は使用できません",
	"validate.mobile":    "%s は有効な携帯電話番号である必要があります",
	"validate.password":  "%s は8文字以上で、大文字・小文字・数字・特殊文字を含む必要があります",
	"validate.username":  "%s は3〜32文字で、英数字とアンダースコアのみ使用できます",
	"validate.no_html":   "%s に HTML タグを含めることはできません",
	"validate.not_blank": "%s は空白にできません",

	// ── 共通ビジネスメッセージ ────────────────────────────────────────────────
	"common.success":          "成功しました",
	"common.created":          "作成しました",
	"common.updated":          "更新しました",
	"common.deleted":          "削除しました",
	"common.not_found":        "リソースが見つかりません",
	"common.unauthorized":     "操作を続けるにはログインしてください",
	"common.forbidden":        "この操作を実行する権限がありません",
	"common.invalid_params":   "リクエストパラメータが無効です",
	"common.server_error":     "内部エラーが発生しました。後ほど再試行してください。",
	"common.rate_limited":     "リクエストが多すぎます。少し待ってください。",
	"common.token_expired":    "セッションが期限切れです。再ログインしてください。",
	"common.token_invalid":    "認証トークンが無効または不足しています",
	"common.bad_content":      "サポートされていないコンテンツタイプです",
	"common.upload_too_large": "ファイルが大きすぎます",
	"common.maintenance":      "サービスはメンテナンス中です。後ほど再試行してください。",
}
