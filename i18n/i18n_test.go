package i18n_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/i18n"
)

// ─── Bundle / Register / Extend ──────────────────────────────────────────────

func TestBundle_Register(t *testing.T) {
	b := i18n.New()
	b.Register("fr", i18n.Messages{
		"common.success": "Succès",
		"http.404":       "Non trouvé",
	})
	tr := b.Translator("fr")
	if got := tr.T("common.success"); got != "Succès" {
		t.Errorf("got %q, want %q", got, "Succès")
	}
}

func TestBundle_Extend(t *testing.T) {
	b := i18n.NewDefault()
	b.Extend("zh", i18n.Messages{
		"app.greeting": "欢迎使用 %s",
	})
	tr := b.Translator("zh")
	// Use a variable for the key so go vet doesn't misidentify T as printf.
	key := "app.greeting"
	if got := tr.T(key, "Astra"); got != "欢迎使用 Astra" {
		t.Errorf("got %q", got)
	}
	// Pre-existing zh key should still work after Extend.
	if got := tr.T("common.success"); got != "操作成功" {
		t.Errorf("existing key broken: got %q", got)
	}
}

func TestBundle_Has(t *testing.T) {
	b := i18n.NewDefault()
	if !b.Has("en") {
		t.Error("expected en to be registered")
	}
	if !b.Has("zh") {
		t.Error("expected zh to be registered")
	}
	if b.Has("xx") {
		t.Error("did not expect xx to be registered")
	}
}

func TestBundle_SetFallback(t *testing.T) {
	b := i18n.New()
	b.Register("zh", i18n.Messages{"http.400": "请求错误"})
	b.SetFallback("zh")

	tr := b.Translator("fr") // fr not registered → fallback zh
	if got := tr.T("http.400"); got != "请求错误" {
		t.Errorf("got %q, want zh fallback", got)
	}
}

// ─── Translator / T ──────────────────────────────────────────────────────────

func TestTranslator_T_FormatsArgs(t *testing.T) {
	b := i18n.NewDefault()
	tr := b.Translator("en")
	// Use a variable so go vet doesn't treat T as a printf-family function.
	key := "validate.min"
	got := tr.T(key, "username", "3")
	want := "username must be at least 3 characters"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTranslator_T_FallsBackToFallbackLocale(t *testing.T) {
	b := i18n.New()
	b.Register("en", i18n.Messages{"http.404": "Not Found"})
	b.SetFallback("en")

	tr := b.Translator("de") // de not registered → fallback en
	if got := tr.T("http.404"); got != "Not Found" {
		t.Errorf("got %q, want %q", got, "Not Found")
	}
}

func TestTranslator_T_FallsBackToKey(t *testing.T) {
	b := i18n.NewDefault()
	tr := b.Translator("en")
	// Unknown key → return key itself.
	key := "app.completely_unknown_key"
	if got := tr.T(key); got != key {
		t.Errorf("got %q, want key itself %q", got, key)
	}
}

func TestTranslator_T_ZHValidationFormatting(t *testing.T) {
	b := i18n.NewDefault()
	tr := b.Translator("zh")
	key := "validate.required"
	got := tr.T(key, "用户名")
	want := "用户名 不能为空"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ─── Built-in locales ─────────────────────────────────────────────────────────

func TestBuiltin_EN(t *testing.T) {
	b := i18n.NewDefault()
	tr := b.Translator("en")
	cases := map[string]string{
		"http.400":         "Bad Request",
		"http.401":         "Unauthorized",
		"http.404":         "Not Found",
		"http.500":         "Internal Server Error",
		"common.success":   "Success",
		"common.created":   "Created successfully",
		"common.not_found": "Resource not found",
	}
	for k, want := range cases {
		if got := tr.T(k); got != want {
			t.Errorf("en[%q]: got %q, want %q", k, got, want)
		}
	}
}

func TestBuiltin_ZH(t *testing.T) {
	b := i18n.NewDefault()
	tr := b.Translator("zh")
	cases := map[string]string{
		"http.400":       "请求参数错误",
		"http.401":       "未授权，请先登录",
		"http.404":       "资源不存在",
		"http.500":       "服务器内部错误，请稍后再试",
		"common.success": "操作成功",
		"common.created": "创建成功",
	}
	for k, want := range cases {
		if got := tr.T(k); got != want {
			t.Errorf("zh[%q]: got %q, want %q", k, got, want)
		}
	}
}

func TestBuiltin_ZH_CN_Alias(t *testing.T) {
	b := i18n.NewDefault()
	if got := b.T("zh-CN", "common.success"); got != "操作成功" {
		t.Errorf("zh-CN common.success: got %q", got)
	}
}

// ─── Accept-Language parsing / DetectLocale ──────────────────────────────────

func TestDetectLocale_QueryParam(t *testing.T) {
	b := i18n.NewDefault()
	if loc := i18n.DetectLocale(b, "zh", "", ""); loc != "zh" {
		t.Errorf("got %q, want zh", loc)
	}
}

func TestDetectLocale_XLanguageHeader(t *testing.T) {
	b := i18n.NewDefault()
	if loc := i18n.DetectLocale(b, "", "zh", ""); loc != "zh" {
		t.Errorf("got %q, want zh", loc)
	}
}

func TestDetectLocale_AcceptLanguage(t *testing.T) {
	b := i18n.NewDefault()
	cases := []struct {
		header string
		want   string
	}{
		{"zh-CN,zh;q=0.9,en;q=0.8", "zh-CN"},
		{"zh;q=0.9,en;q=0.8", "zh"},
		{"en-US,en;q=0.9", "en"},
		{"fr-FR,fr;q=0.9", "fr"}, // fr registered since built-in locales expanded
		{"", "en"},               // empty → fallback
	}
	for _, tc := range cases {
		got := i18n.DetectLocale(b, "", "", tc.header)
		if got != tc.want {
			t.Errorf("Accept-Language=%q: got %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestDetectLocale_Priority(t *testing.T) {
	b := i18n.NewDefault()
	// query param wins over header and Accept-Language.
	loc := i18n.DetectLocale(b, "zh", "en", "en-US,en;q=0.9")
	if loc != "zh" {
		t.Errorf("got %q, want zh (query param should win)", loc)
	}
}

// ─── Global package-level API ─────────────────────────────────────────────────

func TestGlobal_Register(t *testing.T) {
	i18n.Register("test-lang", i18n.Messages{
		"custom.hello": "こんにちは",
	})
	tr := i18n.ForLocale("test-lang")
	if got := tr.T("custom.hello"); got != "こんにちは" {
		t.Errorf("got %q", got)
	}
}

func TestGlobal_Extend(t *testing.T) {
	i18n.Extend("en", i18n.Messages{
		"app.version": "Version %s",
	})
	tr := i18n.ForLocale("en")
	key := "app.version"
	if got := tr.T(key, "1.0"); got != "Version 1.0" {
		t.Errorf("got %q", got)
	}
}

// ─── Middleware + T ───────────────────────────────────────────────────────────

func TestMiddleware_SetsTranslator(t *testing.T) {
	app := astra.New()
	app.Use(i18n.Middleware())
	app.GET("/test", func(c *astra.Ctx) error {
		msg := i18n.T(c, "common.success")
		return c.String(http.StatusOK, "%s", msg)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if got := w.Body.String(); got != "操作成功" {
		t.Errorf("body %q, want 操作成功", got)
	}
}

func TestMiddleware_LangQueryParam(t *testing.T) {
	app := astra.New()
	app.Use(i18n.Middleware())
	app.GET("/test", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", i18n.T(c, "http.404"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test?lang=zh", nil)
	app.ServeHTTP(w, req)

	if got := w.Body.String(); got != "资源不存在" {
		t.Errorf("body %q, want 资源不存在", got)
	}
}

func TestMiddleware_FallsBackToEnWithoutHeader(t *testing.T) {
	app := astra.New()
	app.Use(i18n.Middleware())
	app.GET("/test", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", i18n.T(c, "common.success"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	app.ServeHTTP(w, req)

	if got := w.Body.String(); got != "Success" {
		t.Errorf("body %q, want Success", got)
	}
}

func TestMiddleware_CustomBundle(t *testing.T) {
	bundle := i18n.NewDefault().
		Register("ja", i18n.Messages{
			"common.success": "成功しました",
		}).
		SetFallback("ja")

	app := astra.New()
	app.Use(i18n.Middleware(i18n.MiddlewareConfig{Bundle: bundle}))
	app.GET("/test", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", i18n.T(c, "common.success"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test?lang=ja", nil)
	app.ServeHTTP(w, req)

	if got := w.Body.String(); got != "成功しました" {
		t.Errorf("body %q, want 成功しました", got)
	}
}

func TestT_WithoutMiddleware_UsesFallback(t *testing.T) {
	// Ensure Default fallback is "en".
	i18n.SetFallback("en")

	app := astra.New()
	// No i18n.Middleware mounted.
	app.GET("/test", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", i18n.T(c, "common.success"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	app.ServeHTTP(w, req)

	if got := w.Body.String(); got != "Success" {
		t.Errorf("body %q, want Success (en fallback)", got)
	}
}

func TestGetTranslator(t *testing.T) {
	app := astra.New()
	app.Use(i18n.Middleware())
	app.GET("/test", func(c *astra.Ctx) error {
		tr := i18n.GetTranslator(c)
		if tr == nil {
			t.Error("expected non-nil translator")
		}
		if tr.Locale() != "zh" {
			t.Errorf("locale %q, want zh", tr.Locale())
		}
		return c.String(http.StatusOK, "%s", tr.T("common.created"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Language", "zh")
	app.ServeHTTP(w, req)

	if got := w.Body.String(); got != "创建成功" {
		t.Errorf("body %q, want 创建成功", got)
	}
}

func TestMiddleware_CustomQueryParam(t *testing.T) {
	app := astra.New()
	app.Use(i18n.Middleware(i18n.MiddlewareConfig{QueryParam: "locale"}))
	app.GET("/test", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", i18n.T(c, "common.deleted"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test?locale=zh", nil)
	app.ServeHTTP(w, req)

	if got := w.Body.String(); got != "删除成功" {
		t.Errorf("body %q, want 删除成功", got)
	}
}

// ─── Concurrency ─────────────────────────────────────────────────────────────

func TestBundle_ConcurrentAccess(t *testing.T) {
	b := i18n.NewDefault()
	done := make(chan struct{})

	// Writer goroutine.
	go func() {
		for i := 0; i < 200; i++ {
			b.Extend("en", i18n.Messages{"test.key": "v"})
		}
		close(done)
	}()

	// Reader goroutines.
	for i := 0; i < 4; i++ {
		go func() {
			for j := 0; j < 200; j++ {
				b.Translator("en").T("common.success")
			}
		}()
	}

	<-done
}
