// internal/board/taskfile_test.go
package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 15, 14, 30, 0, 0, time.FixedZone("KST", 9*3600))

func writeTask(t *testing.T, root, id, body string) {
	t.Helper()
	dir := filepath.Join(root, "tasks", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sample = "# VIDEO-07 — 주제 선정\n\n## 메타\n\n```yaml\nstatus: pending\n# 주석 줄\ncreated: 2026-09-15\nupdated: 2026-09-15\nparents: [VIDEO-06, VIDEO-05]\npriority: medium\n```\n\n## 목표\n한 문장.\n"

func TestReadTaskFileParsesTitleStatusParents(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "VIDEO-07", sample)
	tf, err := ReadTaskFile(root, "VIDEO-07")
	if err != nil {
		t.Fatal(err)
	}
	if tf.ID != "VIDEO-07" || tf.Title != "VIDEO-07 — 주제 선정" || tf.Status != "pending" {
		t.Errorf("got %+v", tf)
	}
	if strings.Join(tf.Parents, ",") != "VIDEO-06,VIDEO-05" {
		t.Errorf("parents = %v", tf.Parents)
	}
	if tf.Updated.IsZero() {
		t.Error("Updated(mtime)가 비었다")
	}
}

func TestReadTaskFileParentsMultilineList(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "B", "# B\n```yaml\nstatus: pending\nparents:\n  - A\n  - C\npriority: low\n```\n")
	tf, err := ReadTaskFile(root, "B")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tf.Parents, ",") != "A,C" {
		t.Errorf("parents = %v", tf.Parents)
	}
}

// 인라인 yaml 주석(" #…")은 status:·parents:(한 줄·여러 줄 모두)에서 잘려나가야 한다.
func TestReadTaskFileStripsInlineComments(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "C1", "# C1\n```yaml\nstatus: pending  # 주석\nparents: [A, B]  # 설명\n```\n")
	tf, err := ReadTaskFile(root, "C1")
	if err != nil {
		t.Fatal(err)
	}
	if tf.Status != "pending" {
		t.Errorf("status = %q, want 주석 없이 pending", tf.Status)
	}
	if strings.Join(tf.Parents, ",") != "A,B" {
		t.Errorf("parents = %v, want [A B]", tf.Parents)
	}

	writeTask(t, root, "C2", "# C2\n```yaml\nstatus: pending\nparents:\n  - A  # 설명\n  - B\n```\n")
	tf, err = ReadTaskFile(root, "C2")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tf.Parents, ",") != "A,B" {
		t.Errorf("여러 줄 리스트 parents = %v, want [A B]", tf.Parents)
	}
}

func TestReadTaskFileNoYAMLIsUnknownAndTitleFallsBackToID(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "X", "no heading here\n")
	tf, err := ReadTaskFile(root, "X")
	if err != nil {
		t.Fatal(err)
	}
	if tf.Status != "unknown" || tf.Title != "X" {
		t.Errorf("got %+v", tf)
	}
}

func TestReadTaskFileMissingIsNotExist(t *testing.T) {
	if _, err := ReadTaskFile(t.TempDir(), "NOPE"); !os.IsNotExist(err) {
		t.Errorf("err = %v, want not-exist", err)
	}
}

func TestSetStatusReplacesOnlyStatusAndUpdatedLines(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "VIDEO-07", sample)
	if err := SetStatus(root, "VIDEO-07", "in_progress", now); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "VIDEO-07", "task.md"))
	got := string(b)
	want := strings.Replace(sample, "status: pending", "status: in_progress", 1)
	if got != want {
		t.Errorf("파일 내용이 status/updated 외에 바뀜:\n%s", got)
	}
	if !strings.Contains(got, "updated: 2026-09-15") {
		t.Error("updated: 줄이 사라짐")
	}
	// updated 날짜가 다른 경우 갱신
	writeTask(t, root, "OLD", strings.Replace(sample, "updated: 2026-09-15", "updated: 2026-09-01", 1))
	if err := SetStatus(root, "OLD", "done", now); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(root, "tasks", "OLD", "task.md"))
	if !strings.Contains(string(b), "updated: 2026-09-15") || !strings.Contains(string(b), "status: done") {
		t.Errorf("updated/status 미갱신:\n%s", b)
	}
	// 임시 파일이 남지 않는다
	entries, _ := os.ReadDir(filepath.Join(root, "tasks", "OLD"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".task.md") {
			t.Errorf("임시 파일 잔재: %s", e.Name())
		}
	}
}

func TestSetStatusWithoutYAMLFails(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "X", "# X\nstatus: pending (블록 밖)\n")
	if err := SetStatus(root, "X", "done", now); err != ErrNoStatus {
		t.Errorf("err = %v, want ErrNoStatus", err)
	}
}

func TestAppendLogFormatAndCreatesFile(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", sample)
	if err := AppendLog(root, "A", "ASK", "폴더 밖을 읽어도 될까요?", now); err != nil {
		t.Fatal(err)
	}
	if err := AppendLog(root, "A", "SEND", "네, 됩니다", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "tasks", "A", "log.md"))
	want := "[2026-09-15 14:30] [ASK] 폴더 밖을 읽어도 될까요?\n[2026-09-15 14:31] [SEND] 네, 됩니다\n"
	if !strings.HasSuffix(string(b), want) {
		t.Errorf("log.md:\n%s", b)
	}
	if ReadLastLog(root, "A") != "[2026-09-15 14:31] [SEND] 네, 됩니다" {
		t.Errorf("ReadLastLog = %q", ReadLastLog(root, "A"))
	}
	if ReadLastLog(root, "NOPE") != "" {
		t.Error("없는 log는 빈 문자열")
	}
}

func TestAppendLogFlattensNewlines(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "A", sample)
	if err := AppendLog(root, "A", "SEND", "첫 줄\n둘째 줄", now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ReadLastLog(root, "A"), "첫 줄⏎둘째 줄") {
		t.Errorf("개행이 ⏎로 안 바뀜: %q", ReadLastLog(root, "A"))
	}
}

// 경로 조작 방지: "../x" 같은 업무ID는 ReadTaskFile·SetStatus·AppendLog·ReadLastLog 모두 파일시스템을
// 건드리지 않고 즉시 거부해야 한다(defense in depth — 호출자가 이미 걸러도 여기서 다시 막는다).
func TestBadIDRejectedByAllTaskfileFuncs(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"../x", "..", ".", "", "a/b"} {
		if ValidID(id) {
			t.Errorf("ValidID(%q) = true, want false", id)
		}
		if _, err := ReadTaskFile(root, id); err != ErrBadID {
			t.Errorf("ReadTaskFile(%q) err = %v, want ErrBadID", id, err)
		}
		if err := SetStatus(root, id, "done", now); err != ErrBadID {
			t.Errorf("SetStatus(%q) err = %v, want ErrBadID", id, err)
		}
		if err := AppendLog(root, id, "TAG", "text", now); err != ErrBadID {
			t.Errorf("AppendLog(%q) err = %v, want ErrBadID", id, err)
		}
		if s := ReadLastLog(root, id); s != "" {
			t.Errorf("ReadLastLog(%q) = %q, want \"\"", id, s)
		}
	}
	// 부작용 없음 확인 — root 바깥에 아무것도 생기지 않았다.
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "x")); !os.IsNotExist(err) {
		t.Error("../x가 root 바깥에 파일을 만들었다")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("root 안에 부작용 파일 생성됨: %v", entries)
	}
}

// writeAtomic은 원래 파일의 권한 비트를 보존해야 한다 — os.CreateTemp가 만드는 0600으로
// 조용히 바뀌면 안 된다.
func TestSetStatusPreservesFileMode(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "VIDEO-07", sample)
	p := filepath.Join(root, "tasks", "VIDEO-07", "task.md")
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetStatus(root, "VIDEO-07", "in_progress", now); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", fi.Mode().Perm())
	}
}
