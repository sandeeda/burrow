package fsjail

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// attackPaths are inputs that must never be accepted on any OS.
var attackPaths = []string{
	"..", "../", "../etc/passwd", "a/../../b", "a/..", "./a", "a/./b", "a//b",
	"//server/share", "///etc", `..\..\windows`, `a\b`, `\\server\share`,
	`\\?\C:\Windows`, `\\.\pipe\x`, "a\x00b", "\x00", "a/\x00", "a\nb", "a\tb",
	"a\x7fb", "\xff\xfe", strings.Repeat("a", MaxNameLen+1),
	strings.Repeat("a/", MaxPathLen/2+1),
}

// windowsAttackPaths are additionally rejected on Windows.
var windowsAttackPaths = []string{
	"C:", "C:/Windows", "c:foo", `C:\`, "file.txt:stream", "file.txt::$DATA",
	"CON", "con", "nul.txt", "NUL .txt", "aux.tar.gz", "COM1", "lpt9.log",
	"COM¹", "CONIN$", "CONOUT$", "a/PRN/b", "trailingdot.", "trailingspace ",
	"a./b", "a*b", "a?b", "a<b", "a>b", `a"b`, "a|b",
}

func TestCleanRejectsAttacks(t *testing.T) {
	for _, windows := range []bool{false, true} {
		for _, p := range attackPaths {
			if got, err := clean(p, windows); err == nil {
				t.Errorf("clean(%q, windows=%v) = %q, want error", p, windows, got)
			}
		}
	}
	for _, p := range windowsAttackPaths {
		if got, err := clean(p, true); err == nil {
			t.Errorf("clean(%q, windows) = %q, want error", p, got)
		}
	}
}

func TestCleanAccepts(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "."},
		{"/", "."},
		{"a", "a"},
		{"/a", "a"},
		{"a/", "a"},
		{"a/b/c.txt", filepath.Join("a", "b", "c.txt")},
		{"Photos 2024/IMG_0001.JPG", filepath.Join("Photos 2024", "IMG_0001.JPG")},
		{"..hidden", "..hidden"},
		{"a..b", "a..b"},
		{".config", ".config"},
		{"résumé.pdf", "résumé.pdf"},
		{"日本語/ファイル.txt", filepath.Join("日本語", "ファイル.txt")},
		{"CONSOLE.txt", "CONSOLE.txt"}, // only exact device names are reserved
		{"COM10", "COM10"},
	}
	for _, windows := range []bool{false, true} {
		for _, tt := range tests {
			got, err := clean(tt.in, windows)
			if err != nil {
				t.Errorf("clean(%q, windows=%v) error: %v", tt.in, windows, err)
				continue
			}
			if got != tt.want {
				t.Errorf("clean(%q, windows=%v) = %q, want %q", tt.in, windows, got, tt.want)
			}
		}
	}
}

func TestUnixAllowsWindowsSpecialChars(t *testing.T) {
	// These are legal filenames on Linux/macOS; hiding them would be a bug.
	// On a Windows host filepath.IsLocal rejects some of them anyway, which is
	// the extra safety we want there.
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only behaviour")
	}
	for _, p := range []string{"a:b", "what?.txt", "con", "trailingdot."} {
		if _, err := clean(p, false); err != nil {
			t.Errorf("clean(%q, unix) error: %v", p, err)
		}
	}
}

// FuzzClean checks the invariants of any accepted path, in both OS modes.
func FuzzClean(f *testing.F) {
	seeds := []string{"", "a/b", "photos/x.jpg"}
	seeds = append(seeds, attackPaths...)
	seeds = append(seeds, windowsAttackPaths...)
	for _, p := range seeds {
		f.Add(p)
	}
	f.Fuzz(func(t *testing.T, p string) {
		for _, windows := range []bool{false, true} {
			got, err := clean(p, windows)
			if err != nil {
				continue
			}
			if !filepath.IsLocal(got) {
				t.Fatalf("clean(%q, %v) = %q: not local", p, windows, got)
			}
			if strings.ContainsRune(got, 0) {
				t.Fatalf("clean(%q, %v) = %q: contains NUL", p, windows, got)
			}
			if got == "." {
				continue
			}
			for _, seg := range strings.Split(filepath.ToSlash(got), "/") {
				if seg == "" || seg == "." || seg == ".." {
					t.Fatalf("clean(%q, %v) = %q: bad segment %q", p, windows, got, seg)
				}
				if strings.ContainsRune(seg, '\\') {
					t.Fatalf("clean(%q, %v) = %q: backslash in segment", p, windows, got)
				}
			}
			if windows && strings.ContainsAny(got, `:*?"<>|`) {
				t.Fatalf("clean(%q, windows) = %q: Windows-forbidden char", p, got)
			}
			// Idempotent: cleaning the slash form again yields the same result.
			if again, err := clean(filepath.ToSlash(got), windows); err != nil || again != got {
				t.Fatalf("clean not idempotent: %q -> %q -> %q (%v)", p, got, again, err)
			}
		}
	})
}
