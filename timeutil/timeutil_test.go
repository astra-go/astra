package timeutil_test

import (
	"database/sql/driver"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra/timeutil"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

// restoreConfig saves the current global config and restores it after the test.
func restoreConfig(t *testing.T) {
	t.Helper()
	origLoc := timeutil.Location()
	origLayout := timeutil.Layout()
	t.Cleanup(func() {
		_ = timeutil.SetTimezone(origLoc.String())
		timeutil.SetLayout(origLayout)
	})
}

// ─── Global configuration ─────────────────────────────────────────────────────

func TestSetTimezone_Valid(t *testing.T) {
	restoreConfig(t)
	if err := timeutil.SetTimezone("Asia/Shanghai"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := timeutil.Location().String(); got != "Asia/Shanghai" {
		t.Errorf("Location() = %q, want %q", got, "Asia/Shanghai")
	}
}

func TestSetTimezone_Invalid(t *testing.T) {
	restoreConfig(t)
	if err := timeutil.SetTimezone("Not/AZone"); err == nil {
		t.Fatal("expected error for invalid timezone, got nil")
	}
}

func TestMustSetTimezone_Panics(t *testing.T) {
	restoreConfig(t)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid timezone")
		}
	}()
	timeutil.MustSetTimezone("Bad/Zone")
}

func TestSetLayout_Changes(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout("01/02/2006")
	if got := timeutil.Layout(); got != "01/02/2006" {
		t.Errorf("Layout() = %q, want %q", got, "01/02/2006")
	}
}

func TestDefaultLayout(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout) // reset to known default
	if got := timeutil.Layout(); got != timeutil.DateTimeLayout {
		t.Errorf("default Layout = %q, want %q", got, timeutil.DateTimeLayout)
	}
}

func TestGlobalConfig_ConcurrentReadWrite(t *testing.T) {
	restoreConfig(t)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				timeutil.SetLayout(timeutil.DateTimeLayout)
			} else {
				_ = timeutil.Layout()
				_ = timeutil.Location()
			}
		}(i)
	}
	wg.Wait() // race detector will catch unsynchronised access
}

// ─── Constructors ─────────────────────────────────────────────────────────────

func TestNow_IsValid(t *testing.T) {
	if timeutil.Now().IsZero() {
		t.Fatal("Now() should not be zero")
	}
}

func TestNow_InConfiguredTZ(t *testing.T) {
	restoreConfig(t)
	timeutil.MustSetTimezone("Asia/Shanghai")
	loc := timeutil.Now().Std().Location()
	if loc.String() != "Asia/Shanghai" {
		t.Errorf("Now() location = %q, want Asia/Shanghai", loc.String())
	}
}

func TestUnix_EpochIsValid(t *testing.T) {
	ep := timeutil.Unix(0)
	if ep.IsZero() {
		t.Fatal("Unix(0) should not be zero — epoch is a valid timestamp")
	}
	if ep.Std().Unix() != 0 {
		t.Errorf("Unix(0).Std().Unix() = %d, want 0", ep.Std().Unix())
	}
}

func TestUnix_Negative(t *testing.T) {
	if timeutil.Unix(-1).IsZero() {
		t.Fatal("Unix(-1) should be valid")
	}
}

func TestUnixMilli(t *testing.T) {
	got := timeutil.UnixMilli(1000)
	if got.Std().Unix() != 1 {
		t.Errorf("UnixMilli(1000).Unix() = %d, want 1", got.Std().Unix())
	}
}

func TestFromTime_ZeroReturnsZero(t *testing.T) {
	if !timeutil.FromTime(time.Time{}).IsZero() {
		t.Fatal("FromTime(zero) should be zero")
	}
}

func TestFromTime_Valid(t *testing.T) {
	now := time.Now()
	got := timeutil.FromTime(now)
	if got.IsZero() {
		t.Fatal("FromTime(now) should not be zero")
	}
}

func TestFromTime_InConfiguredTZ(t *testing.T) {
	restoreConfig(t)
	timeutil.MustSetTimezone("Europe/Paris")
	utcTime := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
	got := timeutil.FromTime(utcTime)
	if got.Std().Location().String() != "Europe/Paris" {
		t.Errorf("location = %q, want Europe/Paris", got.Std().Location().String())
	}
	// Instant must be unchanged
	if got.Unix() != utcTime.Unix() {
		t.Errorf("unix changed: got %d, want %d", got.Unix(), utcTime.Unix())
	}
}

func TestParse_ConfiguredLayout(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout("2006/01/02")
	got, err := timeutil.Parse("2024/03/15")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Date() != "2024-03-15" {
		t.Errorf("Date() = %q, want 2024-03-15", got.Date())
	}
}

func TestParse_BadString(t *testing.T) {
	if _, err := timeutil.Parse("not-a-date"); err == nil {
		t.Fatal("expected error for unparseable string")
	}
}

func TestParseLayout_Custom(t *testing.T) {
	got, err := timeutil.ParseLayout(time.RFC3339, "2024-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("ParseLayout RFC3339: %v", err)
	}
	if got.IsZero() {
		t.Fatal("result should not be zero")
	}
}

func TestToday_MidnightInTZ(t *testing.T) {
	restoreConfig(t)
	timeutil.MustSetTimezone("Asia/Tokyo")
	today := timeutil.Today()
	h, m, s := today.Std().Clock()
	if h != 0 || m != 0 || s != 0 {
		t.Errorf("Today() time = %d:%d:%d, want 00:00:00", h, m, s)
	}
	if today.Std().Location().String() != "Asia/Tokyo" {
		t.Errorf("Today() location = %q, want Asia/Tokyo", today.Std().Location().String())
	}
}

// ─── MarshalJSON ──────────────────────────────────────────────────────────────

func TestMarshalJSON_Zero(t *testing.T) {
	var zeroTime timeutil.Time
	b, err := json.Marshal(zeroTime)
	if err != nil {
		t.Fatalf("marshal zero: %v", err)
	}
	if string(b) != "null" {
		t.Errorf("zero Time JSON = %s, want null", b)
	}
}

func TestMarshalJSON_UsesConfiguredLayout(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout)
	// Use a fixed time to make the assertion deterministic.
	ts := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	got, err := json.Marshal(timeutil.FromTime(ts))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `"2024-03-15 10:30:00"`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

func TestMarshalJSON_CustomLayout(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout("2006/01/02")
	ts := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	got, err := json.Marshal(timeutil.FromTime(ts))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != `"2024/03/15"` {
		t.Errorf("JSON = %s, want \"2024/03/15\"", got)
	}
}

func TestMarshalJSON_RoundTrip(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout)
	original := timeutil.FromTime(time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC))
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result timeutil.Time
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Timestamps must match (second precision given the layout)
	if original.Unix() != result.Unix() {
		t.Errorf("round-trip mismatch: original=%d result=%d", original.Unix(), result.Unix())
	}
}

// ─── UnmarshalJSON ────────────────────────────────────────────────────────────

func TestUnmarshalJSON_Null(t *testing.T) {
	var got timeutil.Time
	if err := json.Unmarshal([]byte("null"), &got); err != nil {
		t.Fatalf("unmarshal null: %v", err)
	}
	if !got.IsZero() {
		t.Fatal("null should produce zero Time")
	}
}

func TestUnmarshalJSON_EmptyString(t *testing.T) {
	var got timeutil.Time
	if err := json.Unmarshal([]byte(`""`), &got); err != nil {
		t.Fatalf("unmarshal empty string: %v", err)
	}
	if !got.IsZero() {
		t.Fatal("empty string should produce zero Time")
	}
}

func TestUnmarshalJSON_UnixInt(t *testing.T) {
	var got timeutil.Time
	if err := json.Unmarshal([]byte("1704067200"), &got); err != nil {
		t.Fatalf("unmarshal unix int: %v", err)
	}
	if got.IsZero() {
		t.Fatal("unix int should produce non-zero Time")
	}
	if got.Unix() != 1704067200 {
		t.Errorf("Unix() = %d, want 1704067200", got.Unix())
	}
}

func TestUnmarshalJSON_UnixZeroIsValid(t *testing.T) {
	var got timeutil.Time
	if err := json.Unmarshal([]byte("0"), &got); err != nil {
		t.Fatalf("unmarshal 0: %v", err)
	}
	if got.IsZero() {
		t.Fatal("unix 0 should be valid (not zero/null)")
	}
}

func TestUnmarshalJSON_ConfiguredLayout(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout)
	var got timeutil.Time
	if err := json.Unmarshal([]byte(`"2024-03-15 10:00:00"`), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Date() != "2024-03-15" {
		t.Errorf("Date() = %q, want 2024-03-15", got.Date())
	}
}

func TestUnmarshalJSON_RFC3339Fallback(t *testing.T) {
	restoreConfig(t)
	// Set a non-RFC3339 layout so the fallback must kick in.
	timeutil.SetLayout("01/02/2006")
	var got timeutil.Time
	if err := json.Unmarshal([]byte(`"2024-03-15T10:00:00Z"`), &got); err != nil {
		t.Fatalf("RFC3339 fallback: %v", err)
	}
	if got.IsZero() {
		t.Fatal("RFC3339 fallback should succeed")
	}
}

func TestUnmarshalJSON_DateOnlyFallback(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout)
	var got timeutil.Time
	if err := json.Unmarshal([]byte(`"2024-03-15"`), &got); err != nil {
		t.Fatalf("date-only fallback: %v", err)
	}
	if got.Date() != "2024-03-15" {
		t.Errorf("Date() = %q, want 2024-03-15", got.Date())
	}
}

func TestUnmarshalJSON_BadString(t *testing.T) {
	var got timeutil.Time
	if err := json.Unmarshal([]byte(`"not-a-date"`), &got); err == nil {
		t.Fatal("expected error for unparseable string")
	}
}

// ─── Scan (sql.Scanner) ───────────────────────────────────────────────────────

func TestScan_Nil(t *testing.T) {
	var got timeutil.Time
	if err := got.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if !got.IsZero() {
		t.Fatal("Scan(nil) should produce zero Time")
	}
}

func TestScan_TimeTime(t *testing.T) {
	now := time.Now()
	var got timeutil.Time
	if err := got.Scan(now); err != nil {
		t.Fatalf("Scan(time.Time): %v", err)
	}
	if got.IsZero() {
		t.Fatal("Scan(time.Time) should produce non-zero Time")
	}
}

func TestScan_Bytes(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout)
	var got timeutil.Time
	if err := got.Scan([]byte("2024-03-15 10:00:00")); err != nil {
		t.Fatalf("Scan([]byte): %v", err)
	}
	if got.Date() != "2024-03-15" {
		t.Errorf("Date() = %q, want 2024-03-15", got.Date())
	}
}

func TestScan_String(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout)
	var got timeutil.Time
	if err := got.Scan("2024-03-15 10:00:00"); err != nil {
		t.Fatalf("Scan(string): %v", err)
	}
	if got.Date() != "2024-03-15" {
		t.Errorf("Date() = %q, want 2024-03-15", got.Date())
	}
}

func TestScan_Int64(t *testing.T) {
	var got timeutil.Time
	if err := got.Scan(int64(1704067200)); err != nil {
		t.Fatalf("Scan(int64): %v", err)
	}
	if got.Unix() != 1704067200 {
		t.Errorf("Unix() = %d, want 1704067200", got.Unix())
	}
}

func TestScan_UnknownType_Error(t *testing.T) {
	var got timeutil.Time
	if err := got.Scan(float32(1.0)); err == nil {
		t.Fatal("Scan(float32) should return error")
	}
}

// ─── Value (driver.Valuer) ────────────────────────────────────────────────────

func TestValue_Zero(t *testing.T) {
	var zeroTime timeutil.Time
	v, err := zeroTime.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if v != nil {
		t.Errorf("zero Time Value() = %v, want nil", v)
	}
}

func TestValue_Valid(t *testing.T) {
	now := timeutil.Now()
	v, err := now.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if v == nil {
		t.Fatal("valid Time Value() should not be nil")
	}
	// Must be time.Time (not a string), as drivers expect.
	if _, ok := v.(time.Time); !ok {
		t.Errorf("Value() type = %T, want time.Time", v)
	}
	// Must satisfy driver.Value interface.
	var _ driver.Value = v
}

// ─── Accessors ────────────────────────────────────────────────────────────────

func TestString_Zero(t *testing.T) {
	var zeroTime timeutil.Time
	if s := zeroTime.String(); s != "" {
		t.Errorf("zero.String() = %q, want \"\"", s)
	}
}

func TestString_Valid(t *testing.T) {
	restoreConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout)
	ts := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	got := timeutil.FromTime(ts).String()
	if got != "2024-03-15 10:30:00" {
		t.Errorf("String() = %q, want 2024-03-15 10:30:00", got)
	}
}

func TestDate_Format(t *testing.T) {
	ts := time.Date(2024, 6, 1, 12, 30, 0, 0, time.UTC)
	got := timeutil.FromTime(ts).Date()
	if got != "2024-06-01" {
		t.Errorf("Date() = %q, want 2024-06-01", got)
	}
}

func TestTimeOfDay_Format(t *testing.T) {
	ts := time.Date(2024, 6, 1, 9, 5, 7, 0, time.UTC)
	got := timeutil.FromTime(ts).TimeOfDay()
	if got != "09:05:07" {
		t.Errorf("TimeOfDay() = %q, want 09:05:07", got)
	}
}

func TestIsZero(t *testing.T) {
	var zeroTime timeutil.Time
	if !zeroTime.IsZero() {
		t.Fatal("zero Time.IsZero() should be true")
	}
	if timeutil.Now().IsZero() {
		t.Fatal("Now().IsZero() should be false")
	}
}

// ─── Comparison and arithmetic ────────────────────────────────────────────────

func TestBefore(t *testing.T) {
	t1 := timeutil.Unix(100)
	t2 := timeutil.Unix(200)
	if !t1.Before(t2) {
		t.Error("t1.Before(t2) should be true")
	}
	if t2.Before(t1) {
		t.Error("t2.Before(t1) should be false")
	}
}

func TestAfter(t *testing.T) {
	t1 := timeutil.Unix(100)
	t2 := timeutil.Unix(200)
	if !t2.After(t1) {
		t.Error("t2.After(t1) should be true")
	}
}

func TestEqual(t *testing.T) {
	ts := timeutil.Unix(12345)
	ts2 := timeutil.Unix(12345)
	if !ts.Equal(ts2) {
		t.Error("Equal times should be equal")
	}
}

func TestAdd(t *testing.T) {
	base := timeutil.Unix(1000)
	result := base.Add(time.Hour)
	if result.Unix() != 1000+3600 {
		t.Errorf("Add(1h) = %d, want %d", result.Unix(), 1000+3600)
	}
}

func TestAdd_ZeroTimeUnchanged(t *testing.T) {
	var zeroTime timeutil.Time
	result := zeroTime.Add(time.Hour)
	if !result.IsZero() {
		t.Error("Add on zero Time should return zero Time")
	}
}

func TestSub(t *testing.T) {
	t1 := timeutil.Unix(1000)
	t2 := timeutil.Unix(1000 + 3600)
	d := t2.Sub(t1)
	if d != time.Hour {
		t.Errorf("Sub = %v, want 1h", d)
	}
}

func TestTruncate(t *testing.T) {
	ts := time.Date(2024, 1, 1, 10, 45, 30, 0, time.UTC)
	got := timeutil.FromTime(ts).Truncate(time.Hour)
	if got.TimeOfDay() != "10:00:00" {
		t.Errorf("Truncate(1h) = %q, want 10:00:00", got.TimeOfDay())
	}
}

func TestTruncate_ZeroTimeUnchanged(t *testing.T) {
	var zeroTime timeutil.Time
	result := zeroTime.Truncate(time.Hour)
	if !result.IsZero() {
		t.Error("Truncate on zero Time should return zero Time")
	}
}
