package browser

import (
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Shot은 페이지 전체 스크린샷을 picks/에 저장하고 경로를 돌려준다.
// SavePick과 같은 폴더를 쓰되 -shot 접미사로 요소 캡처와 구분한다.
func Shot(page *rod.Page, stateDir string, now time.Time) (string, error) {
	dir := filepath.Join(stateDir, "picks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := page.Screenshot(true, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, now.Format("20060102-150405")+"-shot.png")
	return path, os.WriteFile(path, b, 0o644)
}
