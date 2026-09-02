package discord

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostFileMultipart(t *testing.T) {
	var gotContent, gotName string
	var gotBytes []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart 아님: %v", err)
			w.WriteHeader(400)
			return
		}
		gotContent = r.FormValue("payload_json")
		f, hdr, err := r.FormFile("files[0]")
		if err != nil {
			t.Errorf("files[0] 없음: %v", err)
			w.WriteHeader(400)
			return
		}
		gotName = hdr.Filename
		gotBytes, _ = io.ReadAll(f)
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()
	err := NewClient(srv.URL).PostFile("스크린샷 https://x", "shot.png", []byte("PNGDATA"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotContent, "스크린샷 https://x") || !strings.Contains(gotContent, "agentlayer") {
		t.Errorf("payload_json: %s", gotContent)
	}
	if gotName != "shot.png" || string(gotBytes) != "PNGDATA" {
		t.Errorf("파일 파트: name=%q bytes=%q", gotName, gotBytes)
	}
}

func TestPostFileErrorHidesWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(413)
		w.Write([]byte(`{"message":"Request entity too large"}`))
	}))
	defer srv.Close()
	err := NewClient(srv.URL+"/api/webhooks/123/SECRET").PostFile("x", "a.png", []byte("1"))
	if err == nil {
		t.Fatal("4xx는 에러여야 함")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Errorf("에러에 웹훅 토큰 노출: %v", err)
	}
	if !strings.Contains(err.Error(), "413") {
		t.Errorf("상태 코드 포함해야 함: %v", err)
	}
}
