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
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.CopyN(f, tr, hdr.Size); err != nil && !errors.Is(err, io.EOF) {
				f.Close()
				return err
			}
			f.Close()
		default:
			return fmt.Errorf("tar 항목 거부(%c): %q", hdr.Typeflag, hdr.Name)
		}
	}
}
