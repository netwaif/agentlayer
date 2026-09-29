package remote

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// 회수 산출물의 실행 비트는 살리되 0755/0644로만 — setuid·setgid·sticky·그룹/기타 쓰기는 버린다.
func TestUntarNormalizesFileMode(t *testing.T) {
	cases := []struct {
		name string
		mode int64
		want os.FileMode
	}{
		{"run.sh", 0o755, 0o755},
		{"suid", 0o4755, 0o755},
		{"sgid-sticky", 0o3777, 0o755},
		{"owner-exec", 0o700, 0o755},
		{"plain", 0o644, 0o644},
		{"world-writable", 0o666, 0o644},
		{"private", 0o600, 0o644},
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, c := range cases {
		if err := tw.WriteHeader(&tar.Header{Name: c.name, Mode: c.mode, Size: 1, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte("x"))
	}
	tw.Close()
	dest := t.TempDir()
	// 이미 있는 파일(넓은 권한)을 덮어써도 모드는 정규화돼야 한다
	if err := os.WriteFile(filepath.Join(dest, "plain"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dest, "plain"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := Untar(dest, &buf, 1<<20); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		fi, err := os.Lstat(filepath.Join(dest, c.name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky); got != c.want {
			t.Errorf("%s(tar %o): mode=%v want %v", c.name, c.mode, got, c.want)
		}
	}
}
