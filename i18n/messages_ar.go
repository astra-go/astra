package i18n

// messagesAR contains the built-in Arabic translations.
// Registered under "ar" by NewDefault.
// Note: Arabic is a right-to-left (RTL) language; UI layers should set
// dir="rtl" when rendering these strings in HTML contexts.
var messagesAR = Messages{
	// ── أخطاء حالة HTTP ──────────────────────────────────────────────────────
	"http.400": "طلب غير صالح",
	"http.401": "غير مصرح به",
	"http.403": "الوصول محظور",
	"http.404": "المورد غير موجود",
	"http.405": "الطريقة غير مسموح بها",
	"http.409": "تعارض في البيانات، يرجى المحاولة مجدداً",
	"http.422": "تنسيق البيانات غير صالح",
	"http.429": "طلبات كثيرة جداً، يرجى المحاولة لاحقاً",
	"http.500": "خطأ داخلي في الخادم",
	"http.503": "الخدمة غير متاحة مؤقتاً",

	// ── أخطاء التحقق على مستوى الحقل ────────────────────────────────────────
	"validate.required":  "%s مطلوب",
	"validate.min":       "يجب أن يحتوي %s على %s أحرف على الأقل",
	"validate.max":       "يجب ألا يتجاوز %s %s أحرف",
	"validate.len":       "يجب أن يحتوي %s على %s أحرف بالضبط",
	"validate.gte":       "يجب أن يكون %s أكبر من أو يساوي %s",
	"validate.lte":       "يجب أن يكون %s أصغر من أو يساوي %s",
	"validate.gt":        "يجب أن يكون %s أكبر من %s",
	"validate.lt":        "يجب أن يكون %s أصغر من %s",
	"validate.email":     "يجب أن يكون %s عنوان بريد إلكتروني صالحاً",
	"validate.url":       "يجب أن يكون %s رابطاً صالحاً",
	"validate.numeric":   "يجب أن يكون %s رقماً",
	"validate.alpha":     "يجب أن يحتوي %s على أحرف فقط",
	"validate.alphanum":  "يجب أن يحتوي %s على أحرف وأرقام فقط",
	"validate.oneof":     "يجب أن يكون %s أحد القيم التالية: %s",
	"validate.unique":    "يجب ألا يحتوي %s على قيم مكررة",
	"validate.mobile":    "يجب أن يكون %s رقم جوال صالحاً",
	"validate.password":  "يجب أن يحتوي %s على 8 أحرف على الأقل، ويتضمن حروفاً كبيرة وصغيرة وأرقاماً وأحرفاً خاصة",
	"validate.username":  "يجب أن يتراوح %s بين 3 و32 حرفاً ويحتوي على أحرف وأرقام وشرطات سفلية فقط",
	"validate.no_html":   "يجب ألا يحتوي %s على وسوم HTML",
	"validate.not_blank": "يجب ألا يكون %s فارغاً",

	// ── رسائل الأعمال الشائعة ────────────────────────────────────────────────
	"common.success":          "تمت العملية بنجاح",
	"common.created":          "تم الإنشاء بنجاح",
	"common.updated":          "تم التحديث بنجاح",
	"common.deleted":          "تم الحذف بنجاح",
	"common.not_found":        "المورد غير موجود",
	"common.unauthorized":     "يرجى تسجيل الدخول للمتابعة",
	"common.forbidden":        "ليس لديك صلاحية لتنفيذ هذا الإجراء",
	"common.invalid_params":   "معاملات الطلب غير صالحة",
	"common.server_error":     "حدث خطأ داخلي. يرجى المحاولة مرة أخرى لاحقاً.",
	"common.rate_limited":     "طلبات كثيرة جداً. يرجى الانتظار قليلاً.",
	"common.token_expired":    "انتهت صلاحية الجلسة. يرجى تسجيل الدخول مرة أخرى.",
	"common.token_invalid":    "رمز المصادقة غير صالح أو مفقود",
	"common.bad_content":      "نوع المحتوى غير مدعوم",
	"common.upload_too_large": "الملف كبير جداً",
	"common.maintenance":      "الخدمة في صيانة. يرجى المحاولة لاحقاً.",
}
