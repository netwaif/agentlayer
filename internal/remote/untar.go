package remote

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// fileMode는 tar 헤더 모드를 0755/0644 둘 중 하나로 줄인다. 소유자 실행 비트만 살린다(회수한 스크립트가
// 그대로 돌게) — setuid·setgid·sticky와 그룹/기타 쓰기 비트는 원격이 만든 것이라 받지 않는다.
func fileMode(hdrMode int64) os.FileMode {
	if hdrMode&0o100 != 0 {
		return 0o755
	}
	return 0o644
}

// Untar는 r의 tar를 destDir 아래에 푼다. 일반 파일·디렉터리만 받고, destDir 밖으로 나가는 경로·심볼릭 링크·
// 상한 초과는 에러. 회수 산출물은 신뢰하지 않는다(원격 에이전트가 만든 것).
func Untar(destDir string, r io.Reader, maxBytes int64) error {
	dest, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	tr := tar.NewReader(r)
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar 읽기: %w", err)
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "" || name == "." {
			continue
		}
		if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
			return fmt.Errorf("tar 경로 거부(절대): %q", hdr.Name)
		}
		target := filepath.Join(dest, name)
		if target != dest && !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return fmt.Errorf("tar 경로 거부(탈출): %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += hdr.Size
			if total > maxBytes {
				return fmt.Errorf("산출물 %dMiB 초과", maxBytes>>20)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := fileMode(hdr.Mode)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.CopyN(f, tr, hdr.Size); err != nil && !errors.Is(err, io.EOF) {
				f.Close()
				return err
			}
			// OpenFile의 모드는 새 파일에만(그것도 umask를 거쳐) 먹는다 — 이미 있던 파일을 덮어써도 같은 모드가 되게.
			if err := f.Chmod(mode); err != nil {
				f.Close()
				return err
			}
			f.Close()
		default:
			return fmt.Errorf("tar 항목 거부(%c): %q", hdr.Typeflag, hdr.Name)
		}
	}
}
