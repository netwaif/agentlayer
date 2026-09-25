package remote

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tarOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	return buf.Bytes()
}

func TestUntarWritesFilesAndDirs(t *testing.T) {
	dest := t.TempDir()
	if err := Untar(dest, bytes.NewReader(tarOf(t, map[string]string{"./a.txt": "A", "sub/b.md": "B"})), 1<<20); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "a.txt")); string(b) != "A" {
		t.Error("a.txt")
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "sub", "b.md")); string(b) != "B" {
		t.Error("sub/b.md")
	}
}

func TestUntarRejectsEscape(t *testing.T) {
	for _, name := range []string{"../evil", "/abs", "sub/../../x"} {
		dest := t.TempDir()
		err := Untar(dest, bytes.NewReader(tarOf(t, map[string]string{name: "x"})), 1<<20)
		if err == nil || !strings.Contains(err.Error(), "경로") {
			t.Errorf("%q: 탈출 경로는 거부해야 함: %v", name, err)
		}
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.Close()
	if err := Untar(t.TempDir(), &buf, 1<<20); err == nil {
		t.Error("symlink 거부")
	}
}

func TestUntarSizeCap(t *testing.T) {
	big := strings.Repeat("x", 2048)
	err := Untar(t.TempDir(), bytes.NewReader(tarOf(t, map[string]string{"big": big})), 1024)
	if err == nil || !strings.Contains(err.Error(), "초과") {
		t.Errorf("상한 초과는 에러: %v", err)
	}
}
