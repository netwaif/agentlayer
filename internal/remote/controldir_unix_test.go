//go:build unix

package remote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureControlDirCreates0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ssh")
	if err := EnsureControlDir(dir); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("없으면 0700으로 만든다: %v %v", fi, err)
	}
	if err := EnsureControlDir(dir); err != nil {
		t.Errorf("내 것·0700이면 통과: %v", err)
	}
}

func TestEnsureControlDirRejectsLooseMode(t *testing.T) {
	for _, mode := range []os.FileMode{0o755, 0o770, 0o777, 0o701} {
		dir := filepath.Join(t.TempDir(), "ssh")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, mode); err != nil {
			t.Fatal(err)
		}
		err := EnsureControlDir(dir)
		if err == nil || !strings.Contains(err.Error(), "권한") {
			t.Errorf("%o: 0700이 아니면 거부: %v", mode, err)
		}
		if fi, _ := os.Lstat(dir); fi.Mode().Perm() != mode {
			t.Errorf("%o: 남의 것일 수 있는 폴더를 고쳐 쓰지 않는다: %v", mode, fi.Mode())
		}
	}
}

func TestEnsureControlDirRejectsSymlinkAndFile(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := EnsureControlDir(link); err == nil || !strings.Contains(err.Error(), "심볼릭 링크") {
		t.Errorf("심볼릭 링크는 가리키는 곳이 멀쩡해도 거부: %v", err)
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureControlDir(file); err == nil {
		t.Error("폴더가 아니면 거부")
	}
}

func TestEnsureControlDirRejectsOtherOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ssh")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// 남의 폴더를 만들 수는 없으니(root 필요) "현재 uid"를 바꿔 같은 상황을 만든다.
	err := ensureControlDir(dir, os.Getuid()+1)
	if err == nil || !strings.Contains(err.Error(), "소유자") {
		t.Errorf("소유자가 다르면 거부: %v", err)
	}
}

// 검증에 실패한 폴더로는 ssh를 띄우지 않는다.
func TestSSHRunnerRefusesBadControlDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ssh")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 검증을 통과해 버려도 실제 ssh는 뜨지 않게
	_, err := SSHRunner{Host: "invalid.invalid", ControlDir: dir}.Run(ctx, nil, "true")
	if err == nil || !strings.Contains(err.Error(), "권한") {
		t.Errorf("ControlDir 검증 실패가 그대로 에러여야 함: %v", err)
	}
}
