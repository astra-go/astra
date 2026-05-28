package pagination_test

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/pagination"
	"github.com/astra-go/astra/testutil"
)

// helper: build a minimal *astra.Ctx with the given query string.
func ctxWithQuery(t *testing.T, query string) *astra.Ctx {
	t.Helper()
	app := testutil.NewTestApp()
	var captured *astra.Ctx
	app.GET("/", func(c *astra.Ctx) error {
		captured = c
		return nil
	})
	req := httptest.NewRequest("GET", "/?"+query, nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if captured == nil {
		t.Fatal("handler was not called")
	}
	return captured
}

// ─── FromRequest ──────────────────────────────────────────────────────────────

func TestFromRequest_Defaults(t *testing.T) {
	c := ctxWithQuery(t, "")
	req := pagination.FromRequest(c)
	testutil.AssertEqual(t, 1, req.Page)
	testutil.AssertEqual(t, 20, req.Size)
	testutil.AssertEqual(t, "", req.Cursor)
}

func TestFromRequest_ExplicitPageAndSize(t *testing.T) {
	c := ctxWithQuery(t, "page=3&size=50")
	req := pagination.FromRequest(c)
	testutil.AssertEqual(t, 3, req.Page)
	testutil.AssertEqual(t, 50, req.Size)
}

func TestFromRequest_SizeClamped_ToMax(t *testing.T) {
	c := ctxWithQuery(t, "size=9999")
	req := pagination.FromRequest(c)
	testutil.AssertEqual(t, 100, req.Size)
}

func TestFromRequest_SizeClamped_ToMin(t *testing.T) {
	c := ctxWithQuery(t, "size=0")
	req := pagination.FromRequest(c)
	testutil.AssertEqual(t, 1, req.Size)
}

func TestFromRequest_PageClamped_ToMin(t *testing.T) {
	c := ctxWithQuery(t, "page=-5")
	req := pagination.FromRequest(c)
	testutil.AssertEqual(t, 1, req.Page)
}

func TestFromRequest_InvalidPage_UsesDefault(t *testing.T) {
	c := ctxWithQuery(t, "page=abc")
	req := pagination.FromRequest(c)
	testutil.AssertEqual(t, 1, req.Page)
}

func TestFromRequest_InvalidSize_UsesDefault(t *testing.T) {
	c := ctxWithQuery(t, "size=xyz")
	req := pagination.FromRequest(c)
	testutil.AssertEqual(t, 20, req.Size)
}

func TestFromRequest_WithCursor(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString([]byte("cursor:42"))
	c := ctxWithQuery(t, "cursor="+token+"&size=10")
	req := pagination.FromRequest(c)
	testutil.AssertEqual(t, token, req.Cursor)
	testutil.AssertEqual(t, 10, req.Size)
}

func TestFromRequest_WithDefaultSize_Option(t *testing.T) {
	c := ctxWithQuery(t, "")
	req := pagination.FromRequest(c, pagination.WithDefaultSize(5))
	testutil.AssertEqual(t, 5, req.Size)
}

func TestFromRequest_WithMaxSize_Option(t *testing.T) {
	c := ctxWithQuery(t, "size=999")
	req := pagination.FromRequest(c, pagination.WithMaxSize(50))
	testutil.AssertEqual(t, 50, req.Size)
}

// ─── Request.Offset ───────────────────────────────────────────────────────────

func TestOffset_Page1(t *testing.T) {
	req := pagination.Request{Page: 1, Size: 20}
	testutil.AssertEqual(t, 0, req.Offset())
}

func TestOffset_Page3(t *testing.T) {
	req := pagination.Request{Page: 3, Size: 20}
	testutil.AssertEqual(t, 40, req.Offset())
}

func TestOffset_PageZero_ReturnsZero(t *testing.T) {
	req := pagination.Request{Page: 0, Size: 20}
	testutil.AssertEqual(t, 0, req.Offset())
}

// ─── Request.DecodeCursor ─────────────────────────────────────────────────────

func TestDecodeCursor_Empty_ReturnsEmpty(t *testing.T) {
	req := pagination.Request{}
	testutil.AssertEqual(t, "", req.DecodeCursor())
}

func TestDecodeCursor_ValidToken(t *testing.T) {
	raw := "id:99"
	token := base64.RawURLEncoding.EncodeToString([]byte(raw))
	req := pagination.Request{Cursor: token}
	testutil.AssertEqual(t, raw, req.DecodeCursor())
}

func TestDecodeCursor_InvalidBase64_ReturnsEmpty(t *testing.T) {
	req := pagination.Request{Cursor: "!!!not-base64!!!"}
	testutil.AssertEqual(t, "", req.DecodeCursor())
}

// ─── NewPage ──────────────────────────────────────────────────────────────────

func TestNewPage_CalculatesPages(t *testing.T) {
	req := pagination.Request{Page: 1, Size: 10}
	page := pagination.NewPage([]int{1, 2, 3}, 25, req)
	testutil.AssertEqual(t, 3, page.Pages) // ceil(25/10)
	testutil.AssertEqual(t, int64(25), page.Total)
	testutil.AssertEqual(t, 1, page.Page)
	testutil.AssertEqual(t, 10, page.Size)
}

func TestNewPage_NilItems_ReturnsEmptySlice(t *testing.T) {
	req := pagination.Request{Page: 1, Size: 10}
	page := pagination.NewPage[int](nil, 0, req)
	if page.Items == nil {
		t.Error("Items should not be nil")
	}
	if len(page.Items) != 0 {
		t.Errorf("Items should be empty, got %v", page.Items)
	}
}

func TestNewPage_ZeroSize_PagesIsZero(t *testing.T) {
	req := pagination.Request{Page: 1, Size: 0}
	page := pagination.NewPage([]string{}, 10, req)
	testutil.AssertEqual(t, 0, page.Pages)
}

func TestNewPage_ExactDivision(t *testing.T) {
	req := pagination.Request{Page: 2, Size: 5}
	page := pagination.NewPage([]string{"a"}, 10, req)
	testutil.AssertEqual(t, 2, page.Pages)
}

// ─── NewCursorPage ────────────────────────────────────────────────────────────

func TestNewCursorPage_HasMore_TrimsItems(t *testing.T) {
	req := pagination.Request{Size: 2}
	// Fetch size+1 = 3 items to detect "has more"
	items := []string{"a", "b", "c"}
	page := pagination.NewCursorPage(items, req, nil)
	testutil.AssertEqual(t, true, page.HasMore)
	testutil.AssertEqual(t, 2, len(page.Items))
}

func TestNewCursorPage_NoMore(t *testing.T) {
	req := pagination.Request{Size: 5}
	items := []string{"a", "b"}
	page := pagination.NewCursorPage(items, req, nil)
	testutil.AssertEqual(t, false, page.HasMore)
	testutil.AssertEqual(t, 2, len(page.Items))
}

func TestNewCursorPage_WithEncodeCursor(t *testing.T) {
	req := pagination.Request{Size: 2}
	items := []string{"a", "b", "c"}
	page := pagination.NewCursorPage(items, req, func(s string) string { return "id:" + s })
	if page.NextCursor == "" {
		t.Error("expected NextCursor to be set when HasMore=true")
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(page.NextCursor)
	testutil.AssertEqual(t, "id:b", string(decoded))
}

func TestNewCursorPage_NilItems_ReturnsEmptySlice(t *testing.T) {
	req := pagination.Request{Size: 10}
	page := pagination.NewCursorPage[string](nil, req, nil)
	if page.Items == nil {
		t.Error("Items should not be nil")
	}
}

func TestNewCursorPage_NoMore_NoCursor(t *testing.T) {
	req := pagination.Request{Size: 5}
	items := []string{"a"}
	page := pagination.NewCursorPage(items, req, func(s string) string { return s })
	testutil.AssertEqual(t, "", page.NextCursor)
}

// ─── EncodeCursor ─────────────────────────────────────────────────────────────

func TestEncodeCursor_RoundTrip(t *testing.T) {
	raw := "user:123"
	encoded := pagination.EncodeCursor(raw)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	testutil.AssertEqual(t, raw, string(decoded))
}

func TestEncodeCursor_Empty(t *testing.T) {
	encoded := pagination.EncodeCursor("")
	testutil.AssertEqual(t, "", encoded)
}
