package browser

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPBKDF2SHA1Vector(t *testing.T) {
	// RFC 6070 테스트 벡터 (P="password", S="salt", c=2, dkLen=20)
	got := pbkdf2SHA1([]byte("password"), []byte("salt"), 2, 20)
	want, _ := hex.DecodeString("ea6c014dc72d6f8ccd1ed92ace1d41f0d8de8957")
	if !bytes.Equal(got, want) {
		t.Fatalf("RFC 벡터 불일치: %x", got)
	}
}

func TestMatchesDomain(t *testing.T) {
	cases := []struct {
		hostKey, domain string
		want            bool
	}{
		{".youtube.com", "youtube.com", true},
		{"youtube.com", "youtube.com", true},
		{"studio.youtube.com", "youtube.com", true},
		{".google.com", "youtube.com", false},
		{"notyoutube.com", "youtube.com", false}, // 접미 경계
		{".youtube.com.evil.com", "youtube.com", false},
	}
	for _, c := range cases {
		if got := matchesDomain(c.hostKey, c.domain); got != c.want {
			t.Errorf("matchesDomain(%q,%q)=%v, want %v", c.hostKey, c.domain, got, c.want)
		}
	}
}

func TestParseHexBlob(t *testing.T) {
	b, err := parseHexBlob("X'763130ABCD'")
	if err != nil || !bytes.Equal(b, []byte{0x76, 0x31, 0x30, 0xAB, 0xCD}) {
		t.Fatalf("hex 파싱: %x %v", b, err)
	}
	if _, err := parseHexBlob("NULL"); err == nil {
		t.Error("NULL은 에러")
	}
}

func TestChromeEpochToTime(t *testing.T) {
	// 2026-01-01 00:00:00 UTC = (11644473600 + 1767225600) * 1e6 마이크로초
	got := chromeEpochToTime((11644473600 + 1767225600) * 1e6)
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("epoch 변환: %v", got)
	}
	if !chromeEpochToTime(0).IsZero() {
		t.Error("0은 세션 쿠키(zero time)")
	}
}

// 테스트가 같은 스킴(v10: AES-128-CBC·IV 공백 16·PKCS7)으로 암호화한 값을
// decryptV10이 복호화하는 라운드트립 — Keychain 불필요.
func encryptV10ForTest(t *testing.T, key []byte, plain []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	iv := bytes.Repeat([]byte{' '}, aes.BlockSize)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return append([]byte("v10"), out...)
}

func TestDecryptV10RoundTrip(t *testing.T) {
	key := pbkdf2SHA1([]byte("테스트비번"), []byte("saltysalt"), 1003, 16)
	enc := encryptV10ForTest(t, key, []byte("cookie-value-123"))
	got, err := decryptV10(key, enc, ".youtube.com")
	if err != nil || got != "cookie-value-123" {
		t.Fatalf("라운드트립: %q %v", got, err)
	}
}

func TestDecryptV10StripsHostHashPrefix(t *testing.T) {
	// 최신 크롬: 평문 앞에 SHA256(host_key) 32바이트 프리픽스
	key := pbkdf2SHA1([]byte("pw"), []byte("saltysalt"), 1003, 16)
	h := sha256.Sum256([]byte(".youtube.com"))
	plain := append(h[:], []byte("real-value")...)
	enc := encryptV10ForTest(t, key, plain)
	got, err := decryptV10(key, enc, ".youtube.com")
	if err != nil || got != "real-value" {
		t.Fatalf("프리픽스 제거: %q %v", got, err)
	}
}

func TestDecryptV10RejectsUnknownScheme(t *testing.T) {
	key := pbkdf2SHA1([]byte("pw"), []byte("saltysalt"), 1003, 16)
	if _, err := decryptV10(key, []byte("v20xxxx"), "a.com"); err == nil {
		t.Error("v10 외 스킴은 에러")
	}
}

func TestParseCookieRows(t *testing.T) {
	out := ".youtube.com\tSID\tX'763130'\t/\t13400000000000000\t1\t1\t1\n" +
		"broken-line-without-tabs\n" +
		".google.com\tNID\tX'763130'\t/\t0\t0\t0\t-1\n"
	rows := parseCookieRows([]byte(out))
	if len(rows) != 2 {
		t.Fatalf("정상 행 2개 파싱해야: %d", len(rows))
	}
	r := rows[0]
	if r.HostKey != ".youtube.com" || r.Name != "SID" || r.Path != "/" ||
		!r.Secure || !r.HTTPOnly || r.SameSite != 1 || r.ExpiresUTC != 13400000000000000 {
		t.Fatalf("행 파싱: %+v", r)
	}
}

func TestBuildCookieParamsFiltersAndDecrypts(t *testing.T) {
	key := pbkdf2SHA1([]byte("pw"), []byte("saltysalt"), 1003, 16)
	blob := func(host, val string) string {
		return "X'" + hex.EncodeToString(encryptV10ForTest(t, key, []byte(val))) + "'"
	}
	future := (11644473600 + 4102444800) * int64(1e6) // 2100년경
	past := (11644473600 + 946684800) * int64(1e6)    // 2000년경
	rows := []cookieRow{
		{HostKey: ".youtube.com", Name: "SID", EncValue: blob(".youtube.com", "sid-val"), Path: "/", ExpiresUTC: future, Secure: true, HTTPOnly: true, SameSite: 2},
		{HostKey: ".google.com", Name: "NID", EncValue: blob(".google.com", "nid"), Path: "/", ExpiresUTC: 0},      // 세션 쿠키·미매칭
		{HostKey: ".youtube.com", Name: "OLD", EncValue: blob(".youtube.com", "old"), Path: "/", ExpiresUTC: past}, // 만료
	}
	params, counts, skipped := buildCookieParams(rows, key, []string{"youtube.com"}, time.Now())
	if len(params) != 1 || counts["youtube.com"] != 1 || skipped != 1 {
		t.Fatalf("params=%d counts=%v skipped=%d", len(params), counts, skipped)
	}
	p := params[0]
	if p.Name != "SID" || p.Value != "sid-val" || p.Domain != ".youtube.com" ||
		!p.Secure || !p.HTTPOnly || p.SameSite != "Strict" {
		t.Fatalf("주입 파라미터: %+v", p)
	}
}

func TestSameSiteLabel(t *testing.T) {
	for in, want := range map[int]string{2: "Strict", 1: "Lax", 0: "None", -1: ""} {
		if got := sameSiteLabel(in); got != want {
			t.Errorf("sameSiteLabel(%d)=%q, want %q", in, got, want)
		}
	}
}

// 실사용 Chrome은 프로필이 여러 개(계정별)라 Default만 읽으면 남의 계정 쿠키를 가져온다.
// Local State에서 프로필 목록을 읽고, 도메인 쿠키가 가장 많은 프로필을 고른다.
func TestListChromeProfiles(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Local State"), []byte(`{"profile":{"info_cache":{
		"Default":{"name":"사용자 이름 1","user_name":"a@x.com"},
		"Profile 1":{"name":"netwaif","user_name":"b@x.com"}},"last_used":"Profile 1"}}`), 0o600)
	ps, last := ListChromeProfiles(home)
	if len(ps) != 2 || last != "Profile 1" {
		t.Fatalf("profiles=%v last=%q", ps, last)
	}
	if ps[1].Dir != "Profile 1" || ps[1].Name != "netwaif" {
		t.Errorf("정렬·이름: %+v", ps)
	}
}

func TestChooseProfileMostCookies(t *testing.T) {
	ps := []ChromeProfile{{Dir: "Default", Name: "a"}, {Dir: "Profile 1", Name: "b"}, {Dir: "Profile 4", Name: "c"}}
	counts := map[string]int{"Default": 5, "Profile 1": 16, "Profile 4": 0}
	got := ChooseProfile(ps, func(d string) int { return counts[d] }, "Default")
	if got.Dir != "Profile 1" {
		t.Errorf("쿠키 많은 프로필 선택: %+v", got)
	}
	// 동률이면 마지막 사용 프로필
	counts["Default"] = 16
	if got := ChooseProfile(ps, func(d string) int { return counts[d] }, "Default"); got.Dir != "Default" {
		t.Errorf("동률은 last_used: %+v", got)
	}
	// 지정 프로필(디렉터리명 또는 표시 이름)
	if got := ChooseProfile(ps, nil, "", "c"); got.Dir != "Profile 4" {
		t.Errorf("이름 지정: %+v", got)
	}
}
