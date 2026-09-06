package browser

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

var exportNow = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func future(d time.Duration) proto.TimeSinceEpoch {
	return proto.TimeSinceEpoch(exportNow.Add(d).Unix())
}

func TestPickCookiePrefersExactHost(t *testing.T) {
	all := []*proto.NetworkCookie{
		{Name: "sessionKey", Value: "sub", Domain: "api.claude.ai", Expires: future(48 * time.Hour)},
		{Name: "sessionKey", Value: "exact", Domain: "claude.ai", Expires: future(time.Hour)},
	}
	c, err := pickCookie(all, "claude.ai", "sessionKey", exportNow)
	if err != nil {
		t.Fatal(err)
	}
	if c.Value != "exact" {
		t.Errorf("정확히 같은 호스트를 골라야 함: %+v", c)
	}
}

func TestPickCookieFallsBackToLongestExpiry(t *testing.T) {
	all := []*proto.NetworkCookie{
		{Name: "k", Value: "short", Domain: "a.x.com", Expires: future(time.Hour)},
		{Name: "k", Value: "long", Domain: ".b.x.com", Expires: future(72 * time.Hour)},
	}
	c, err := pickCookie(all, "x.com", "k", exportNow)
	if err != nil {
		t.Fatal(err)
	}
	if c.Value != "long" {
		t.Errorf("만료가 가장 먼 것을 골라야 함: %+v", c)
	}
}

func TestPickCookieSkipsExpiredAndErrorsWhenNone(t *testing.T) {
	all := []*proto.NetworkCookie{
		{Name: "k", Value: "old", Domain: "x.com", Expires: proto.TimeSinceEpoch(exportNow.Add(-time.Hour).Unix())},
		{Name: "other", Value: "v", Domain: "x.com"},
	}
	if _, err := pickCookie(all, "x.com", "k", exportNow); err == nil {
		t.Fatal("만료된 것뿐이면 에러여야 함")
	} else if !strings.Contains(err.Error(), "cookies import") {
		t.Errorf("import 안내가 있어야 함: %v", err)
	}
}

func TestExportableCookiesFiltersDomainAndExpiry(t *testing.T) {
	all := []*proto.NetworkCookie{
		{Name: "a", Domain: ".youtube.com", Expires: future(time.Hour)},
		{Name: "b", Domain: "accounts.youtube.com"}, // 세션 쿠키(Expires 0)도 포함
		{Name: "c", Domain: "youtube.com", Expires: proto.TimeSinceEpoch(exportNow.Add(-time.Minute).Unix())},
		{Name: "d", Domain: "google.com", Expires: future(time.Hour)},
	}
	got := exportableCookies(all, "youtube.com", exportNow)
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Errorf("a·b만 남아야 함: %+v", got)
	}
}

func TestFormatNetscape(t *testing.T) {
	cs := []*proto.NetworkCookie{
		{Name: "SID", Value: "abc", Domain: ".youtube.com", Path: "/", Secure: true, Expires: future(time.Hour)},
		{Name: "sess", Value: "z", Domain: "accounts.youtube.com", Path: "/x", HTTPOnly: true},
	}
	out := FormatNetscape(cs)
	if !strings.HasPrefix(out, "# Netscape HTTP Cookie File\n") {
		t.Fatalf("헤더 없음:\n%s", out)
	}
	exp := exportNow.Add(time.Hour).Unix()
	want1 := ".youtube.com\tTRUE\t/\tTRUE\t" + itoa(exp) + "\tSID\tabc"
	want2 := "#HttpOnly_accounts.youtube.com\tFALSE\t/x\tFALSE\t0\tsess\tz"
	if !strings.Contains(out, want1+"\n") {
		t.Errorf("도메인 쿠키 줄 틀림:\n%s", out)
	}
	if !strings.Contains(out, want2+"\n") {
		t.Errorf("HttpOnly 호스트 쿠키 줄 틀림:\n%s", out)
	}
}

func TestFormatCookieJSON(t *testing.T) {
	cs := []*proto.NetworkCookie{
		{Name: "k", Value: "v", Domain: "x.com", Path: "/", Secure: true, HTTPOnly: true, Expires: future(time.Hour)},
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(FormatCookieJSON(cs)), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["name"] != "k" || got[0]["value"] != "v" || got[0]["domain"] != "x.com" ||
		got[0]["secure"] != true || got[0]["httpOnly"] != true || got[0]["expires"] != float64(exportNow.Add(time.Hour).Unix()) {
		t.Errorf("JSON 필드 틀림: %+v", got)
	}
}

func TestWriteSecretFileIsOwnerOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "key.txt")
	if err := writeSecretFile(p, "value\n"); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("0600이어야 함: %o", st.Mode().Perm())
	}
	if b, _ := os.ReadFile(p); string(b) != "value\n" {
		t.Errorf("내용 틀림: %q", b)
	}
}

func TestUpdateEnvFileReplacesAddsAndPreserves(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("A=1\n# 주석\nKEY=old\nB=2"), 0o644)
	if err := updateEnvFile(p, "KEY", "new"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "A=1\n# 주석\nKEY=new\nB=2\n" {
		t.Errorf("KEY 줄만 바뀌고 나머지는 보존돼야 함:\n%s", b)
	}
	if err := updateEnvFile(p, "NEW", "x"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if !strings.HasSuffix(string(b), "B=2\nNEW=x\n") {
		t.Errorf("없는 키는 끝에 추가돼야 함:\n%s", b)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("0600이어야 함: %o", st.Mode().Perm())
	}
}

func TestUpdateEnvFileCreatesWhenMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new", ".env")
	if err := updateEnvFile(p, "K", "v"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "K=v\n" {
		t.Errorf("새 파일: %q", b)
	}
}

func TestUpdateEnvFileQuotesValuesWithSpecials(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if err := updateEnvFile(p, "K", "a b#c"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "K=\"a b#c\"\n" {
		t.Errorf("공백·# 포함 값은 따옴표로 감싸야 함: %q", b)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := expandHome("~/x/.env"); got != filepath.Join(home, "x/.env") {
		t.Errorf("~ 확장 틀림: %s", got)
	}
	if got := expandHome("/abs"); got != "/abs" {
		t.Errorf("절대경로는 그대로: %s", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// 실브라우저: 값은 파일에만 가고 출력에는 절대 안 나와야 한다. value·env·netscape 세 경로.
func TestExportCookiesIntegration(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chrome 없음")
	}
	SetHeadlessForTest(true)
	defer SetHeadlessForTest(false)
	dir := t.TempDir()
	b, err := Connect(dir, freePort(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { b.MustClose(); cleanupProfile(dir) }()
	exp := proto.TimeSinceEpoch(float64(time.Now().Add(time.Hour).Unix()))
	seed := []*proto.NetworkCookieParam{
		{Name: "sessionKey", Value: "sk-ant-SECRET", Domain: "claude.ai", Path: "/", Secure: true, HTTPOnly: true, Expires: exp},
		{Name: "other", Value: "o", Domain: ".claude.ai", Path: "/", Secure: true, Expires: exp},
		{Name: "c", Value: "3", Domain: "example.com", Path: "/", Secure: true, Expires: exp},
	}
	if err := (proto.StorageSetCookies{Cookies: seed}).Call(b); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	work := t.TempDir()

	var out bytes.Buffer
	keyFile := filepath.Join(work, "key.txt")
	if err := ExportCookies(b, ExportOptions{Domain: "claude.ai", Name: "sessionKey", To: keyFile}, now, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got, _ := os.ReadFile(keyFile); string(got) != "sk-ant-SECRET\n" {
		t.Errorf("value 파일: %q", got)
	}
	if strings.Contains(out.String(), "SECRET") {
		t.Fatalf("출력에 값이 새면 안 됨: %s", out.String())
	}
	if !strings.Contains(out.String(), "sessionKey") || !strings.Contains(out.String(), keyFile) {
		t.Errorf("요약 출력: %s", out.String())
	}

	out.Reset()
	envFile := filepath.Join(work, ".env")
	os.WriteFile(envFile, []byte("PORT=1\nCLAUDE_SESSION_KEY=old\n"), 0o644)
	if err := ExportCookies(b, ExportOptions{Domain: "claude.ai", Name: "sessionKey", EnvFile: envFile, EnvKey: "CLAUDE_SESSION_KEY"}, now, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got, _ := os.ReadFile(envFile); string(got) != "PORT=1\nCLAUDE_SESSION_KEY=sk-ant-SECRET\n" {
		t.Errorf(".env: %q", got)
	}
	if strings.Contains(out.String(), "SECRET") {
		t.Fatalf("출력에 값이 새면 안 됨: %s", out.String())
	}

	out.Reset()
	txt := filepath.Join(work, "cookies.txt")
	if err := ExportCookies(b, ExportOptions{Domain: "claude.ai", To: txt}, now, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	got, _ := os.ReadFile(txt)
	if !strings.Contains(string(got), "#HttpOnly_claude.ai\tFALSE\t/\tTRUE\t") || !strings.Contains(string(got), "\tother\to\n") || strings.Contains(string(got), "example.com") {
		t.Errorf("netscape 파일:\n%s", got)
	}
	if !strings.Contains(out.String(), "쿠키 2개") {
		t.Errorf("요약 출력: %s", out.String())
	}

	if err := ExportCookies(b, ExportOptions{Domain: "claude.ai", Name: "sessionKey"}, now, &out); err == nil {
		t.Error("--to/--env 없이 stdout으로 내보내기는 거부돼야 함")
	}
}

func TestMissingAfterSet(t *testing.T) {
	params := []*proto.NetworkCookieParam{
		{Name: "a", Domain: ".x.com", Path: "/"},
		{Name: "b", Domain: "api.x.com", Path: "/"},
		{Name: "b", Domain: "api.x.com", Path: "/v2"},
	}
	present := []*proto.NetworkCookie{
		{Name: "a", Domain: ".x.com", Path: "/"},
		{Name: "b", Domain: "api.x.com", Path: "/v2"},
	}
	got := missingAfterSet(params, present)
	if len(got) != 1 || got[0] != "b@api.x.com/" {
		t.Errorf("주입 안 된 것만 이름@호스트경로로: %v", got)
	}
}

// 실브라우저: Chrome이 조용히 거부하는 쿠키(SameSite=None인데 Secure 아님)를 import가 이름으로 보고해야 한다.
func TestInjectCookiesReportsRejected(t *testing.T) {
	if _, ok := launcher.LookPath(); !ok {
		t.Skip("Chrome 없음")
	}
	SetHeadlessForTest(true)
	defer SetHeadlessForTest(false)
	dir := t.TempDir()
	b, err := Connect(dir, freePort(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { b.MustClose(); cleanupProfile(dir) }()
	exp := proto.TimeSinceEpoch(float64(time.Now().Add(time.Hour).Unix()))
	params := []*proto.NetworkCookieParam{
		{Name: "ok", Value: "1", Domain: ".x.com", Path: "/", Secure: true, Expires: exp},
		{Name: "bad", Value: "2", Domain: ".x.com", Path: "/", Secure: false, SameSite: "None", Expires: exp},
	}
	rejected, err := injectCookies(b, params)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 1 || rejected[0] != "bad@.x.com/" {
		t.Errorf("거부된 쿠키 보고: %v", rejected)
	}
}
