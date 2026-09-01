package browser

// cookies.go — 실사용 크롬의 지정 도메인 쿠키만 골라 에이전트 전용
// 브라우저 프로필로 가져온다. 전체 프로필 복사가 아니라 도메인 화이트리스트
// 방식이라 격리 취지(에이전트에게 줄 세션만 명시적으로 들여옴)를 지킨다.
// macOS 전용 — 크롬 Cookies DB(SQLite) + Keychain "Chrome Safe Storage" 키로
// v10 복호화 후 CDP로 주입한다.

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// cookieRow는 크롬 Cookies 테이블 한 행 (조회 필드만).
type cookieRow struct {
	HostKey    string
	Name       string
	EncValue   string // quote(encrypted_value) 결과 X'..' hex 문자열
	Path       string
	ExpiresUTC int64
	Secure     bool
	HTTPOnly   bool
	SameSite   int
}

// pbkdf2SHA1은 표준 라이브러리만으로 PBKDF2-HMAC-SHA1을 계산한다 (의존성 회피).
func pbkdf2SHA1(password, salt []byte, iter, keyLen int) []byte {
	prf := func(data []byte) []byte {
		h := hmac.New(sha1.New, password)
		h.Write(data)
		return h.Sum(nil)
	}
	hashLen := sha1.Size
	blocks := (keyLen + hashLen - 1) / hashLen
	var dk []byte
	for b := 1; b <= blocks; b++ {
		msg := append(append([]byte{}, salt...), byte(b>>24), byte(b>>16), byte(b>>8), byte(b))
		u := prf(msg)
		t := append([]byte{}, u...)
		for i := 2; i <= iter; i++ {
			u = prf(u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

// matchesDomain은 host_key가 요청 도메인(자신 또는 하위 도메인)에 속하는지 본다.
// 앞 점(.youtube.com)은 무시하고 경계(".")를 지켜 notyoutube.com 오매칭을 막는다.
func matchesDomain(hostKey, domain string) bool {
	h := strings.TrimPrefix(hostKey, ".")
	return h == domain || strings.HasSuffix(h, "."+domain)
}

// parseHexBlob은 sqlite3 quote() 출력 X'..'를 바이트로 되돌린다.
func parseHexBlob(s string) ([]byte, error) {
	if !strings.HasPrefix(s, "X'") || !strings.HasSuffix(s, "'") || len(s) < 3 {
		return nil, fmt.Errorf("hex blob 형식 아님: %q", s)
	}
	return hex.DecodeString(s[2 : len(s)-1])
}

// chromeEpochToTime은 크롬 epoch(1601-01-01 기준 마이크로초)를 시각으로.
// 0은 세션 쿠키(만료 없음)라 zero time으로 돌려준다.
func chromeEpochToTime(micro int64) time.Time {
	if micro == 0 {
		return time.Time{}
	}
	const epochDiff = 11644473600 // 1601→1970 초
	return time.Unix(micro/1e6-epochDiff, (micro%1e6)*1000).UTC()
}

// decryptV10은 macOS 크롬 "v10" 쿠키를 복호화한다.
// AES-128-CBC, IV=공백 16바이트, PKCS7. 최신 크롬은 평문 앞에
// SHA256(host_key) 32바이트 프리픽스를 붙이므로 일치하면 제거한다.
func decryptV10(key, enc []byte, hostKey string) (string, error) {
	if len(enc) < 3 || string(enc[:3]) != "v10" {
		return "", fmt.Errorf("v10 스킴이 아닙니다")
	}
	ct := enc[3:]
	if len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return "", fmt.Errorf("암호문 길이 불량")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	iv := bytes.Repeat([]byte{' '}, aes.BlockSize)
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ct)
	pad := int(out[len(out)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(out) {
		return "", fmt.Errorf("패딩 불량")
	}
	out = out[:len(out)-pad]
	if len(out) >= 32 {
		h := sha256.Sum256([]byte(hostKey))
		if bytes.Equal(out[:32], h[:]) {
			out = out[32:]
		}
	}
	return string(out), nil
}

// parseCookieRows는 sqlite3(-separator 탭) 출력을 파싱한다. 필드 수가
// 안 맞는 줄은 조용히 건너뛴다.
func parseCookieRows(out []byte) []cookieRow {
	var rows []cookieRow
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 8 {
			continue
		}
		exp, _ := strconv.ParseInt(f[4], 10, 64)
		ss, _ := strconv.Atoi(f[7])
		rows = append(rows, cookieRow{
			HostKey: f[0], Name: f[1], EncValue: f[2], Path: f[3],
			ExpiresUTC: exp, Secure: f[5] == "1", HTTPOnly: f[6] == "1", SameSite: ss,
		})
	}
	return rows
}

// sameSiteLabel은 크롬 samesite 정수를 CDP 라벨로. -1(미지정)은 빈 값.
func sameSiteLabel(v int) string {
	switch v {
	case 2:
		return "Strict"
	case 1:
		return "Lax"
	case 0:
		return "None"
	default:
		return ""
	}
}

// chromeCookiesPath는 기본 프로필 Cookies DB 경로.
func chromeCookiesPath(home string) string {
	return filepath.Join(home, "Library", "Application Support",
		"Google", "Chrome", "Default", "Cookies")
}

// buildCookieParams는 도메인에 매칭되는 쿠키만 복호화해 CDP 파라미터로 만든다.
// 만료·복호화 실패는 건너뛰고 skipped로 센다. (순수 로직 — 테스트 대상)
func buildCookieParams(rows []cookieRow, key []byte, domains []string, now time.Time) ([]*proto.NetworkCookieParam, map[string]int, int) {
	counts := map[string]int{}
	skipped := 0
	var params []*proto.NetworkCookieParam
	for _, r := range rows {
		matched := ""
		for _, d := range domains {
			if matchesDomain(r.HostKey, d) {
				matched = d
				break
			}
		}
		if matched == "" {
			continue
		}
		if r.ExpiresUTC != 0 && chromeEpochToTime(r.ExpiresUTC).Before(now) {
			skipped++
			continue
		}
		raw, err := parseHexBlob(r.EncValue)
		if err != nil {
			skipped++
			continue
		}
		val, err := decryptV10(key, raw, r.HostKey)
		if err != nil {
			skipped++
			continue
		}
		p := &proto.NetworkCookieParam{
			Name: r.Name, Value: val, Domain: r.HostKey, Path: r.Path,
			Secure: r.Secure, HTTPOnly: r.HTTPOnly,
		}
		if ss := sameSiteLabel(r.SameSite); ss != "" {
			p.SameSite = proto.NetworkCookieSameSite(ss)
		}
		if r.ExpiresUTC != 0 {
			p.Expires = proto.TimeSinceEpoch(chromeEpochToTime(r.ExpiresUTC).Unix())
		}
		params = append(params, p)
		counts[matched]++
	}
	return params, counts, skipped
}

// readChromeCookies는 크롬이 잠근 DB를 임시로 복사(WAL 포함)해 조회한다.
func readChromeCookies(sqlite3Path, dbPath string) ([]cookieRow, error) {
	if sqlite3Path == "" {
		sqlite3Path = "sqlite3"
		if _, err := exec.LookPath(sqlite3Path); err != nil {
			sqlite3Path = "/usr/bin/sqlite3"
		}
	}
	tmp, err := os.MkdirTemp("", "agentlayer-cookies")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	dst := filepath.Join(tmp, "Cookies")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if b, err := os.ReadFile(dbPath + suffix); err == nil {
			if err := os.WriteFile(dst+suffix, b, 0o600); err != nil {
				return nil, err
			}
		}
	}
	if _, err := os.Stat(dst); err != nil {
		return nil, fmt.Errorf("크롬 쿠키 DB를 찾을 수 없습니다: %s", dbPath)
	}
	const q = "SELECT host_key, name, quote(encrypted_value), path, expires_utc, is_secure, is_httponly, samesite FROM cookies"
	out, err := exec.Command(sqlite3Path, "-separator", "\t", dst, q).Output()
	if err != nil {
		return nil, fmt.Errorf("쿠키 DB 조회 실패: %w", err)
	}
	return parseCookieRows(out), nil
}

// safeStoragePassword는 Keychain에서 크롬 쿠키 복호화 키를 읽는다.
// macOS가 접근 허용 팝업을 띄울 수 있다(호출 전에 안내 출력).
func safeStoragePassword() ([]byte, error) {
	out, err := exec.Command("security", "find-generic-password", "-w",
		"-s", "Chrome Safe Storage").Output()
	if err != nil {
		return nil, fmt.Errorf("Keychain에서 Chrome Safe Storage 키를 읽지 못했습니다 " +
			"(권한을 거부했거나 크롬 미설치)")
	}
	return bytes.TrimRight(out, "\n"), nil
}

// ImportCookies는 전 과정을 엮는다: Keychain 키 → DB 조회 → 도메인 필터·복호화
// → CDP 주입 → 개수 보고. 쿠키 값은 절대 출력하지 않는다(개수·도메인만).
func ImportCookies(b *rod.Browser, home, sqlite3Path string, domains []string, now time.Time, out io.Writer) error {
	fmt.Fprintln(out, "Keychain 접근 권한을 요청합니다 — macOS 팝업이 뜨면 '항상 허용'을 눌러주세요…")
	pw, err := safeStoragePassword()
	if err != nil {
		return err
	}
	key := pbkdf2SHA1(pw, []byte("saltysalt"), 1003, 16)
	rows, err := readChromeCookies(sqlite3Path, chromeCookiesPath(home))
	if err != nil {
		return err
	}
	params, counts, skipped := buildCookieParams(rows, key, domains, now)
	if len(params) == 0 {
		return fmt.Errorf("가져올 유효한 쿠키가 없습니다 " +
			"(도메인 철자 또는 크롬에서 해당 사이트 로그인 여부를 확인하세요)")
	}
	if err := (proto.StorageSetCookies{Cookies: params}).Call(b); err != nil {
		return fmt.Errorf("쿠키 주입 실패: %w", err)
	}
	for _, d := range domains {
		fmt.Fprintf(out, "%s: 쿠키 %d개 가져옴\n", d, counts[d])
	}
	if skipped > 0 {
		fmt.Fprintf(out, "(만료·복호화 불가 %d개 건너뜀)\n", skipped)
	}
	fmt.Fprintln(out, "완료 — 에이전트 브라우저에서 해당 사이트에 로그인 상태로 접속됩니다.")
	return nil
}
