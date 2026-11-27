package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sandeeda/burrow/internal/fsjail"
)

func newIndex(t *testing.T, files map[string]string) (*Index, string) {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	j, err := fsjail.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	ix, err := Open(filepath.Join(t.TempDir(), "index.db"), j)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })
	if err := ix.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return ix, dir
}

func paths(rs []Result) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Path)
	}
	return out
}

func search(t *testing.T, ix *Index, q string) []string {
	t.Helper()
	rs, err := ix.Search(context.Background(), q, 100)
	if err != nil {
		t.Fatal(err)
	}
	return paths(rs)
}

func TestSearch(t *testing.T) {
	ix, _ := newIndex(t, map[string]string{
		"Taxes/2024/receipt-march.pdf": "x",
		"Taxes/2023/receipt.pdf":       "x",
		"Photos/beach.JPG":             "x",
		"notes_100%.txt":               "x",
		"notesX100Y.txt":               "x",
		".burrow-upload-123.tmp":       "x",
	})
	tests := map[string][]string{
		"beach":         {"Photos/beach.JPG"},
		"BEACH":         {"Photos/beach.JPG"},
		"receipt 2024":  nil, // words match the name only, not the folder
		"receipt march": {"Taxes/2024/receipt-march.pdf"},
		"taxes":         {"Taxes"},
		"100%":          {"notes_100%.txt"}, // % is literal, not a wildcard
		"_100":          {"notes_100%.txt"}, // _ is literal too
		"burrow-upload": nil,                // temp files never indexed
		"":              nil,
		"   ":           nil,
	}
	for q, want := range tests {
		got := search(t, ix, q)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("Search(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestSearchFoldersFirst(t *testing.T) {
	ix, _ := newIndex(t, map[string]string{"report.txt": "x", "report/inner.txt": "x"})
	got := search(t, ix, "report")
	if len(got) != 2 || got[0] != "report" {
		t.Errorf("got %v, want folder first", got)
	}
}

func TestRescanPicksUpChanges(t *testing.T) {
	ix, dir := newIndex(t, map[string]string{"old.txt": "x"})
	os.Remove(filepath.Join(dir, "old.txt"))
	os.WriteFile(filepath.Join(dir, "new.txt"), nil, 0o644)
	if err := ix.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := search(t, ix, "old"); got != nil {
		t.Errorf("old.txt still indexed: %v", got)
	}
	if got := search(t, ix, "new"); len(got) != 1 {
		t.Errorf("new.txt not indexed: %v", got)
	}
}

func TestUpdateAndRemove(t *testing.T) {
	ix, dir := newIndex(t, map[string]string{"a/b/c.txt": "x", "ab.txt": "x"})
	ctx := context.Background()

	// Simulate a folder rename a -> z.
	if err := os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "z")); err != nil {
		t.Fatal(err)
	}
	if err := ix.Update(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := ix.Update(ctx, "z"); err != nil {
		t.Fatal(err)
	}
	if got := search(t, ix, "c.txt"); fmt.Sprint(got) != "[z/b/c.txt]" {
		t.Errorf("after rename: %v", got)
	}
	// Removing "a" must not touch "ab.txt" (prefix match is per folder).
	if got := search(t, ix, "ab"); fmt.Sprint(got) != "[ab.txt]" {
		t.Errorf("ab.txt affected: %v", got)
	}

	if err := ix.Remove(ctx, "z"); err != nil {
		t.Fatal(err)
	}
	if got := search(t, ix, "c"); fmt.Sprint(got) != "[]" && got != nil {
		t.Errorf("after remove: %v", got)
	}
}

func TestUpdateSurvivesNextScan(t *testing.T) {
	ix, dir := newIndex(t, map[string]string{})
	os.WriteFile(filepath.Join(dir, "fresh.txt"), nil, 0o644)
	ctx := context.Background()
	if err := ix.Update(ctx, "fresh.txt"); err != nil {
		t.Fatal(err)
	}
	if err := ix.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if got := search(t, ix, "fresh"); len(got) != 1 {
		t.Errorf("fresh.txt lost: %v", got)
	}
}

func TestReopenKeepsIndex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), nil, 0o644)
	j, _ := fsjail.Open(dir)
	defer j.Close()
	db := filepath.Join(t.TempDir(), "index.db")
	ix, err := Open(db, j)
	if err != nil {
		t.Fatal(err)
	}
	ix.Scan(context.Background())
	ix.Close()

	ix, err = Open(db, j)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	if got := search(t, ix, "keep"); len(got) != 1 {
		t.Errorf("index not persisted: %v", got)
	}
}
