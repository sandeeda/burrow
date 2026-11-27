package fsjail

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

const secret = "TOP-SECRET-OUTSIDE-JAIL"

// setup builds:
//
//	parent/
//	  secret.txt          <- must never be readable
//	  root/               <- the jail
//	    inside.txt
//	    sub/file.txt
//	    escape-link       -> ../secret.txt    (if symlinks are supported)
//	    escape-dir        -> ..               (if symlinks are supported)
//	    abs-link          -> <abs>/secret.txt (if symlinks are supported)
//	    ok-link           -> inside.txt       (if symlinks are supported)
func setup(t testing.TB) (*Jail, bool) {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	must(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	must(t, os.WriteFile(filepath.Join(parent, "secret.txt"), []byte(secret), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "inside.txt"), []byte("inside"), 0o644))
	must(t, os.WriteFile(filepath.Join(root, "sub", "file.txt"), []byte("sub"), 0o644))

	symlinks := os.Symlink(filepath.Join("..", "secret.txt"), filepath.Join(root, "escape-link")) == nil
	if symlinks {
		must(t, os.Symlink("..", filepath.Join(root, "escape-dir")))
		must(t, os.Symlink(filepath.Join(parent, "secret.txt"), filepath.Join(root, "abs-link")))
		must(t, os.Symlink("inside.txt", filepath.Join(root, "ok-link")))
	}

	j, err := Open(root)
	must(t, err)
	t.Cleanup(func() { j.Close() })
	return j, symlinks
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func readVia(j *Jail, p string) (string, error) {
	f, err := j.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	return string(b), err
}

func TestJailReadsInside(t *testing.T) {
	j, _ := setup(t)
	for p, want := range map[string]string{"inside.txt": "inside", "/sub/file.txt": "sub"} {
		got, err := readVia(j, p)
		if err != nil || got != want {
			t.Errorf("read %q = %q, %v; want %q", p, got, err, want)
		}
	}
	entries, err := j.ReadDir("")
	if err != nil || len(entries) < 2 {
		t.Errorf("ReadDir root = %d entries, %v", len(entries), err)
	}
}

func TestJailBlocksEscapes(t *testing.T) {
	j, symlinks := setup(t)
	paths := append([]string{}, attackPaths...)
	paths = append(paths, windowsAttackPaths...)
	paths = append(paths, "../secret.txt", "sub/../../secret.txt", filepath.Join(filepath.Dir(j.Dir()), "secret.txt"))
	if symlinks {
		paths = append(paths, "escape-link", "escape-dir/secret.txt", "abs-link")
	}
	for _, p := range paths {
		if got, err := readVia(j, p); err == nil && got == secret {
			t.Errorf("read %q leaked the secret", p)
		}
		if fi, err := j.Stat(p); err == nil && fi.Name() == "secret.txt" {
			t.Errorf("stat %q reached the secret", p)
		}
	}
}

func TestJailInternalSymlinkWorks(t *testing.T) {
	j, symlinks := setup(t)
	if !symlinks {
		t.Skip("symlinks not supported here")
	}
	if got, err := readVia(j, "ok-link"); err != nil || got != "inside" {
		t.Errorf("read ok-link = %q, %v", got, err)
	}
}

func TestJailWritesStayInside(t *testing.T) {
	j, symlinks := setup(t)
	parent := filepath.Dir(j.Dir())

	if err := j.Mkdir("../evil"); err == nil {
		t.Error("Mkdir ../evil succeeded")
	}
	if f, err := j.OpenFile("../evil.txt", os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		f.Close()
		t.Error("create ../evil.txt succeeded")
	}
	if err := j.Rename("inside.txt", "../moved.txt"); err == nil {
		t.Error("rename out of jail succeeded")
	}
	if err := j.Remove("../secret.txt"); err == nil {
		t.Error("remove ../secret.txt succeeded")
	}
	if symlinks {
		if f, err := j.OpenFile("escape-dir/evil.txt", os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.Close()
			t.Error("create via escaping symlink succeeded")
		}
	}
	for _, name := range []string{"evil", "evil.txt", "moved.txt"} {
		if _, err := os.Stat(filepath.Join(parent, name)); err == nil {
			t.Errorf("%s was created outside the jail", name)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "secret.txt")); err != nil {
		t.Errorf("secret.txt was touched: %v", err)
	}
}

func TestJailRootIsProtected(t *testing.T) {
	j, _ := setup(t)
	for _, p := range []string{"", "/"} {
		if err := j.Remove(p); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Remove(%q) = %v, want ErrInvalidPath", p, err)
		}
		if err := j.Rename(p, "x"); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Rename(%q, x) = %v, want ErrInvalidPath", p, err)
		}
		if err := j.Mkdir(p); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Mkdir(%q) = %v, want ErrInvalidPath", p, err)
		}
	}
}

func TestJailRenameNoOverwrite(t *testing.T) {
	j, _ := setup(t)
	err := j.Rename("sub/file.txt", "inside.txt")
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("Rename onto existing = %v, want ErrExist", err)
	}
	if got, _ := readVia(j, "inside.txt"); got != "inside" {
		t.Errorf("inside.txt was overwritten: %q", got)
	}
	must(t, j.Rename("sub/file.txt", "renamed.txt"))
	if got, _ := readVia(j, "renamed.txt"); got != "sub" {
		t.Errorf("renamed.txt = %q", got)
	}
}

// FuzzJailOpen throws arbitrary paths at a real jail and checks the secret
// outside it is never read, even through symlinks.
func FuzzJailOpen(f *testing.F) {
	seeds := []string{"inside.txt", "escape-link", "escape-dir/secret.txt", "abs-link"}
	seeds = append(seeds, attackPaths...)
	seeds = append(seeds, windowsAttackPaths...)
	for _, p := range seeds {
		f.Add(p)
	}
	j, _ := setup(f)
	f.Fuzz(func(t *testing.T, p string) {
		if got, err := readVia(j, p); err == nil && got == secret {
			t.Fatalf("read %q leaked the secret", p)
		}
		if fi, err := j.Stat(p); err == nil && fi.Name() == "secret.txt" {
			t.Fatalf("stat %q reached the secret", p)
		}
	})
}
