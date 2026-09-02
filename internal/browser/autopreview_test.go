package browser

import (
	"testing"
	"time"
)

type apFake struct {
	servers []DevServer
	tabs    map[int]bool
	opened  []int
}

func (f *apFake) deps() (func(map[string]string) []DevServer, func(int) bool, func(DevServer) error) {
	return func(map[string]string) []DevServer { return f.servers },
		func(p int) bool { return f.tabs[p] },
		func(s DevServer) error { f.opened = append(f.opened, s.Port); return nil }
}

func TestAutoPreviewOpensOnceAndReopensAfterRestart(t *testing.T) {
	dir := t.TempDir()
	f := &apFake{servers: []DevServer{{Port: 8000, CWD: "/w/app"}}, tabs: map[int]bool{}}
	scan, hasTab, open := f.deps()
	t0 := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)
	AutoPreview(dir, map[string]string{"/w/app": ""}, scan, hasTab, open, t0)
	AutoPreview(dir, map[string]string{"/w/app": ""}, scan, hasTab, open, t0.Add(10*time.Second)) // 같은 서버
	if len(f.opened) != 1 {
		t.Fatalf("한 번만 열어야 함: %v", f.opened)
	}
	f.servers = nil // 서버 내려감
	AutoPreview(dir, map[string]string{"/w/app": ""}, scan, hasTab, open, t0.Add(20*time.Second))
	f.servers = []DevServer{{Port: 8000, CWD: "/w/app"}} // 재시작
	AutoPreview(dir, map[string]string{"/w/app": ""}, scan, hasTab, open, t0.Add(30*time.Second))
	if len(f.opened) != 2 {
		t.Errorf("재시작한 서버는 다시 열어야 함: %v", f.opened)
	}
}

func TestAutoPreviewSkipsExistingTabButMarksSeen(t *testing.T) {
	dir := t.TempDir()
	f := &apFake{servers: []DevServer{{Port: 8000, CWD: "/w/app"}}, tabs: map[int]bool{8000: true}}
	scan, hasTab, open := f.deps()
	t0 := time.Now()
	AutoPreview(dir, map[string]string{"/w/app": ""}, scan, hasTab, open, t0)
	f.tabs = map[int]bool{} // 사용자가 탭을 닫음
	AutoPreview(dir, map[string]string{"/w/app": ""}, scan, hasTab, open, t0.Add(10*time.Second))
	if len(f.opened) != 0 {
		t.Errorf("이미 본 서버는 탭을 닫아도 되살리지 않음: %v", f.opened)
	}
}

// hook마다 불리므로 5초 안에는 스캔 자체를 건너뛴다.
func TestAutoPreviewThrottles(t *testing.T) {
	dir := t.TempDir()
	scans := 0
	f := &apFake{tabs: map[int]bool{}}
	_, hasTab, open := f.deps()
	scan := func(map[string]string) []DevServer { scans++; return nil }
	t0 := time.Now()
	AutoPreview(dir, map[string]string{"/w": ""}, scan, hasTab, open, t0)
	AutoPreview(dir, map[string]string{"/w": ""}, scan, hasTab, open, t0.Add(2*time.Second))
	AutoPreview(dir, map[string]string{"/w": ""}, scan, hasTab, open, t0.Add(6*time.Second))
	if scans != 2 {
		t.Errorf("스캔 횟수 %d, want 2 (5초 스로틀)", scans)
	}
}
