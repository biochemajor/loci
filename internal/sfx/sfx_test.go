package sfx

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTree creates a small source tree and returns its root plus the source
// paths to pack.
func writeTree(t *testing.T) (root string, sources []string) {
	t.Helper()
	root = t.TempDir()
	mustWrite(t, filepath.Join(root, "report.pdf"), "hello world\n")
	mustWrite(t, filepath.Join(root, "notes.txt"), "line1\nline2\n")
	mustWrite(t, filepath.Join(root, "docs", "inner.md"), "nested\n")
	return root, []string{
		filepath.Join(root, "report.pdf"),
		filepath.Join(root, "notes.txt"),
		filepath.Join(root, "docs"),
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func packAndExtract(t *testing.T, opts Options, password string) *ExtractResult {
	t.Helper()
	container, res, err := BuildContainer(opts)
	if err != nil {
		t.Fatalf("BuildContainer: %v", err)
	}
	if len(res.Files) == 0 {
		t.Fatal("no files recorded")
	}

	// Emulate a real SFX file: some stub bytes, container, trailer.
	stub := []byte("PRETEND-STUB-BINARY")
	sfxPath := filepath.Join(t.TempDir(), "out.bin")
	if err := WriteSFX(stub, container, sfxPath, false); err != nil {
		t.Fatalf("WriteSFX: %v", err)
	}

	arc, err := Load(sfxPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "extracted")
	er, err := arc.Extract(dest, password)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return er
}

func TestRoundTripPlain(t *testing.T) {
	_, sources := writeTree(t)
	er := packAndExtract(t, Options{
		Sources: sources,
		Rename:  Rename{Prefix: "SECURE_", Suffix: "_v2"},
	}, "")

	want := map[string]bool{
		"SECURE_report_v2.pdf":    false,
		"SECURE_notes_v2.txt":     false,
		"docs/SECURE_inner_v2.md": false,
	}
	for _, f := range er.Files {
		if !f.Verified {
			t.Errorf("%s not verified", f.Original)
		}
		rel, _ := filepath.Rel(er.DestDir, f.Written)
		want[filepath.ToSlash(rel)] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("missing expected output %q", name)
		}
	}
}

func TestRoundTripEncrypted(t *testing.T) {
	_, sources := writeTree(t)
	const pw = "Sw0rdfish!"
	er := packAndExtract(t, Options{Sources: sources, Password: pw}, pw)
	for _, f := range er.Files {
		if !f.Verified {
			t.Errorf("%s not verified", f.Original)
		}
	}
}

func TestWrongPassword(t *testing.T) {
	_, sources := writeTree(t)
	container, _, err := BuildContainer(Options{Sources: sources, Password: "correct-horse"})
	if err != nil {
		t.Fatal(err)
	}
	sfxPath := filepath.Join(t.TempDir(), "out.bin")
	if err := WriteSFX([]byte("STUB"), container, sfxPath, false); err != nil {
		t.Fatal(err)
	}
	arc, err := Load(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arc.Extract(filepath.Join(t.TempDir(), "x"), "wrong"); err != ErrWrongPassword {
		t.Fatalf("expected ErrWrongPassword, got %v", err)
	}
}

func TestTamperDetected(t *testing.T) {
	_, sources := writeTree(t)
	container, _, err := BuildContainer(Options{Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte inside the blob region (well past the header).
	container[len(container)-1] ^= 0xFF
	sfxPath := filepath.Join(t.TempDir(), "out.bin")
	if err := WriteSFX([]byte("STUB"), container, sfxPath, false); err != nil {
		t.Fatal(err)
	}
	arc, err := Load(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arc.Extract(filepath.Join(t.TempDir(), "x"), ""); err == nil {
		t.Fatal("expected corruption to be detected, got nil error")
	}
}

func TestSafeRename(t *testing.T) {
	cases := []struct {
		name, prefix, suffix, want string
	}{
		{"a.txt", "p_", "_s", "p_a_s.txt"},
		{"a.txt", "", "", "a.txt"},
		{"dir/b.md", "x", "", "dir/xb.md"},
		{"archive.tar.gz", "", "_1", "archive.tar_1.gz"},
		{"noext", "pre_", "_post", "pre_noext_post"},
	}
	for _, c := range cases {
		got, err := SafeRename(c.name, Rename{Prefix: c.prefix, Suffix: c.suffix})
		if err != nil {
			t.Errorf("SafeRename(%q): %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("SafeRename(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestNoSources(t *testing.T) {
	if _, _, err := BuildContainer(Options{}); err == nil {
		t.Fatal("expected error for empty sources")
	}
}
