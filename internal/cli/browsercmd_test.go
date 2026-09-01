package cli

import (
	"bytes"
	"strings"
	"testing"
)

// 미지 서브커맨드는 명확한 에러로 알린다 — Task 6~8이 case를 추가해도
// default 분기의 문구는 유지돼야 한다.
func TestRunBrowserUnknownSub(t *testing.T) {
	var buf bytes.Buffer
	err := RunBrowser(&buf, []string{"없는명령"})
	if err == nil || !strings.Contains(err.Error(), "없는명령") {
		t.Fatalf("미지 서브커맨드는 에러: %v", err)
	}
	if !strings.Contains(err.Error(), "help") {
		t.Errorf("에러에 help 안내가 없다: %v", err)
	}
}
