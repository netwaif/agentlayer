package remote

import (
	"testing"
	"time"
)

func hermesRemote() Remote {
	return Remote{Name: "hermes-qa", Kind: "hermes", SSH: "hostinger",
		Exec:    []string{"docker", "exec", "-i", "-u", "hermes", "hermes-agent-iqxn-hermes-agent-1"},
		Profile: "tech-qa", WorkspaceRoot: "/opt/data/ai-company/결과물"}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{"hermes-qa": true, "a": true, "with:colon": false, "../x": false, "": false, "a b": false} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q)=%v want %v", name, got, want)
		}
	}
}

func TestValidateAndDefaults(t *testing.T) {
	r := hermesRemote()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.PollInterval() != 5*time.Second {
		t.Errorf("기본 poll 5s, got %v", r.PollInterval())
	}
	bad := hermesRemote()
	bad.WorkspaceRoot = ""
	if bad.Validate() == nil {
		t.Error("hermes는 workspace_root 필수")
	}
	ex := Remote{Name: "oc", Kind: "exec", Commands: map[string][]string{"dispatch": {"./d.sh"}, "poll": {"./p.sh"}}}
	if err := ex.Validate(); err != nil {
		t.Errorf("exec는 dispatch·poll만 있으면 유효: %v", err)
	}
	ex.Commands = map[string][]string{"poll": {"./p.sh"}}
	if ex.Validate() == nil {
		t.Error("exec는 dispatch 필수")
	}
}

func TestSaveLoadListDelete(t *testing.T) {
	dir := t.TempDir()
	r := hermesRemote()
	if err := Save(dir, r); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(dir, "hermes-qa")
	if err != nil || !ok || got.Profile != "tech-qa" || got.Mailbox != "imac-manager" {
		t.Fatalf("Load: %+v ok=%v err=%v (Mailbox 기본값 imac-manager)", got, ok, err)
	}
	if _, ok, _ := Load(dir, "none"); ok {
		t.Error("없는 이름은 ok=false")
	}
	if _, _, err := Load(dir, "bad:name"); err == nil {
		t.Error("잘못된 이름은 에러")
	}
	list, _ := List(dir)
	if len(list) != 1 || list[0].Name != "hermes-qa" {
		t.Errorf("List=%+v", list)
	}
	if found, _ := Delete(dir, "hermes-qa"); !found {
		t.Error("Delete found=false")
	}
	if list, _ := List(dir); len(list) != 0 {
		t.Error("삭제 뒤 비어야 함")
	}
}
