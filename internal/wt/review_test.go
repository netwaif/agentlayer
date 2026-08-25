package wt

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTestRecordsResult(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	m, _ := New(stateDir, NewOptions{Task: "t", Repo: repo, NoWindow: true, TestCmd: "true"})
	var buf bytes.Buffer
	pass, err := RunTest(&buf, stateDir, "t", "")
	if err != nil || !pass {
		t.Fatalf("true는 통과: %v %v", pass, err)
	}
	m, _ = LoadMeta(stateDir, "t")
	if m.TestPass == nil || !*m.TestPass || m.TestAt == nil {
		t.Errorf("결과 기록: %+v", m)
	}
	// 실패 명령 + override 저장
	pass, err = RunTest(&buf, stateDir, "t", "false")
	if err != nil || pass {
		t.Fatalf("false는 실패: %v %v", pass, err)
	}
	m, _ = LoadMeta(stateDir, "t")
	if *m.TestPass || m.TestCmd != "false" {
		t.Errorf("실패 기록 + cmd override: %+v", m)
	}
}

func TestRunTestNoCmd(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	New(stateDir, NewOptions{Task: "t", Repo: repo, NoWindow: true})
	if _, err := RunTest(&bytes.Buffer{}, stateDir, "t", ""); err == nil {
		t.Error("명령 없으면 에러")
	}
}

func TestReviewRoundTrip(t *testing.T) {
	repo := fixtureRepo(t)
	stateDir := t.TempDir()
	m, _ := New(stateDir, NewOptions{Task: "t", Repo: repo, NoWindow: true})

	// 변경 없으면 리뷰 거부
	if _, err := WriteReviewFile(stateDir, "t"); err == nil {
		t.Error("변경 없으면 리뷰 파일 안 만듦")
	}

	os.WriteFile(filepath.Join(m.Path, "a.txt"), []byte("변경된 내용\n"), 0o644)
	path, err := WriteReviewFile(stateDir, "t")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "wt send t") || !strings.Contains(string(content), "+변경된 내용") {
		t.Errorf("리뷰 파일 내용:\n%s", content)
	}

	// 코멘트 삽입 후 추출
	edited := strings.Replace(string(content), "+변경된 내용",
		"+변경된 내용\n#> 이 줄은 상수로 빼주세요\n#> 그리고 테스트 추가", 1)
	os.WriteFile(path, []byte(edited), 0o644)
	comments, err := ExtractComments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 {
		t.Fatalf("코멘트 2건: %+v", comments)
	}
	if comments[0].Text != "이 줄은 상수로 빼주세요" || !strings.Contains(comments[0].Context, "변경된 내용") {
		t.Errorf("코멘트+문맥: %+v", comments[0])
	}
	inst := BuildInstruction("t", comments)
	if !strings.Contains(inst, "2건") || !strings.Contains(inst, "상수로") || strings.Contains(inst, "\n") {
		t.Errorf("지시 문단은 한 줄: %q", inst)
	}
}
