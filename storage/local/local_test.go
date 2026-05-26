package local_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/astra-go/astra/storage"
	"github.com/astra-go/astra/storage/local"
)

func newStore(t *testing.T) *local.Store {
	t.Helper()
	s, err := local.New(local.Config{RootDir: t.TempDir(), BaseURL: "http://localhost/files"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestPutGet(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if err := s.Put(ctx, "a/b.txt", strings.NewReader("hello"), storage.PutOptions{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rc, err := s.Get(ctx, "a/b.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", data, "hello")
	}
}

func TestExistsDelete(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	s.Put(ctx, "x.txt", strings.NewReader("x"), storage.PutOptions{})

	ok, err := s.Exists(ctx, "x.txt")
	if err != nil || !ok {
		t.Fatalf("Exists: %v %v", ok, err)
	}
	if err := s.Delete(ctx, "x.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ok, _ = s.Exists(ctx, "x.txt")
	if ok {
		t.Fatal("expected not exists after delete")
	}
	// Delete non-existent should not error.
	if err := s.Delete(ctx, "x.txt"); err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}
}

func TestStat(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	s.Put(ctx, "stat.txt", strings.NewReader("hello"), storage.PutOptions{})

	info, err := s.Stat(ctx, "stat.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size != 5 {
		t.Fatalf("Size: got %d, want 5", info.Size)
	}
}

func TestSignedURL(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	s.Put(ctx, "img/foo.png", strings.NewReader("data"), storage.PutOptions{})

	u, err := s.SignedURL(ctx, "img/foo.png", time.Minute)
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if !strings.HasPrefix(u, "http://localhost/files/") {
		t.Fatalf("unexpected URL: %s", u)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	for _, k := range []string{"a/1.txt", "a/2.txt", "b/3.txt"} {
		s.Put(ctx, k, strings.NewReader("x"), storage.PutOptions{})
	}

	res, err := s.List(ctx, "a/", "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Objects) != 2 {
		t.Fatalf("got %d objects, want 2", len(res.Objects))
	}
}

func TestListPagination(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	for i := range 5 {
		s.Put(ctx, strings.Repeat("x", i+1), strings.NewReader("v"), storage.PutOptions{})
	}

	res1, _ := s.List(ctx, "", "", 3)
	if len(res1.Objects) != 3 || !res1.IsTruncated {
		t.Fatalf("page1: got %d objects, truncated=%v", len(res1.Objects), res1.IsTruncated)
	}
	res2, _ := s.List(ctx, "", res1.NextToken, 3)
	if len(res2.Objects) != 2 {
		t.Fatalf("page2: got %d objects, want 2", len(res2.Objects))
	}
}

func TestCopy(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	s.Put(ctx, "src.txt", strings.NewReader("content"), storage.PutOptions{})

	if err := s.Copy(ctx, "src.txt", "dst.txt"); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	rc, _ := s.Get(ctx, "dst.txt")
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "content" {
		t.Fatalf("Copy content mismatch: %q", data)
	}
}

func TestMultipart(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	uploadID, err := s.CreateMultipartUpload(ctx, "multi.bin", storage.PutOptions{})
	if err != nil {
		t.Fatalf("CreateMultipartUpload: %v", err)
	}

	part1 := bytes.Repeat([]byte("A"), 64)
	part2 := bytes.Repeat([]byte("B"), 32)

	etag1, err := s.UploadPart(ctx, "multi.bin", uploadID, 1, bytes.NewReader(part1), int64(len(part1)))
	if err != nil {
		t.Fatalf("UploadPart 1: %v", err)
	}
	etag2, err := s.UploadPart(ctx, "multi.bin", uploadID, 2, bytes.NewReader(part2), int64(len(part2)))
	if err != nil {
		t.Fatalf("UploadPart 2: %v", err)
	}

	err = s.CompleteMultipartUpload(ctx, "multi.bin", uploadID, []storage.CompletedPart{
		{PartNumber: 1, ETag: etag1},
		{PartNumber: 2, ETag: etag2},
	})
	if err != nil {
		t.Fatalf("CompleteMultipartUpload: %v", err)
	}

	info, _ := s.Stat(ctx, "multi.bin")
	if info.Size != int64(len(part1)+len(part2)) {
		t.Fatalf("size: got %d, want %d", info.Size, len(part1)+len(part2))
	}
}

func TestAbortMultipart(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	uploadID, _ := s.CreateMultipartUpload(ctx, "abort.bin", storage.PutOptions{})
	s.UploadPart(ctx, "abort.bin", uploadID, 1, strings.NewReader("data"), 4)

	if err := s.AbortMultipartUpload(ctx, "abort.bin", uploadID); err != nil {
		t.Fatalf("AbortMultipartUpload: %v", err)
	}
	// Object should not exist after abort.
	ok, _ := s.Exists(ctx, "abort.bin")
	if ok {
		t.Fatal("object should not exist after abort")
	}
}

func TestPathTraversal(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if err := s.Put(ctx, "../escape.txt", strings.NewReader("x"), storage.PutOptions{}); err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}
