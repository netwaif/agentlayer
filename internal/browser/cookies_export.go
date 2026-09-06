package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// cookies export — 전용 프로필의 쿠키를 값 그대로 "파일로만" 흘려보낸다.
// 사용량 모니터·yt-dlp·봇 설정처럼 "브라우저에선 되는데 자동화만 하면 로그인에서 막히는"
// 도구에 세션을 넘겨주는 통로. 값은 stdout·로그·Discord 어디에도 찍지 않는다(list의 정책 유지).

// ExportFormat은 파일에 쓰는 형식.
type ExportFormat string

const (
	FormatValue     ExportFormat = "value"    // 쿠키 하나의 값 한 줄
	FormatNetscapeF ExportFormat = "netscape" // cookies.txt (yt-dlp --cookies, curl -b)
	FormatJSONF     ExportFormat = "json"     // 이름·값·호스트·만료 배열
)

// ExportOptions는 export 한 번의 지시. To와 EnvFile 중 하나만 쓴다.
type ExportOptions struct {
	Domain  string
	Name    string       // 비면 도메인 전체
	Format  ExportFormat // 비면 Name 유무로 value/netscape 결정
	To      string       // 결과 파일 (~ 허용)
	EnvFile string       // .env 파일 — Name 필수, EnvKey=값 줄을 갱신
	EnvKey  string
}

func notExpired(c *proto.NetworkCookie, now time.Time) bool {
	return c.Expires <= 0 || !time.Unix(int64(c.Expires), 0).Before(now)
}

// exportableCookies는 도메인(하위 포함)에 속하고 만료되지 않은 쿠키를 호스트·이름 순으로 돌려준다.
func exportableCookies(all []*proto.NetworkCookie, domain string, now time.Time) []*proto.NetworkCookie {
	var out []*proto.NetworkCookie
	for _, c := range all {
		if matchesDomain(c.Domain, domain) && notExpired(c, now) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// pickCookie는 이름이 같은 쿠키가 여러 호스트에 있을 때 하나를 고른다:
// 요청 도메인과 정확히 같은 호스트 우선, 없으면 만료가 가장 먼 것(세션 쿠키는 가장 먼 것으로 친다).
func pickCookie(all []*proto.NetworkCookie, domain, name string, now time.Time) (*proto.NetworkCookie, error) {
	var best *proto.NetworkCookie
	for _, c := range exportableCookies(all, domain, now) {
		if c.Name != name {
			continue
		}
		host := strings.TrimPrefix(c.Domain, ".")
		if host == domain {
			return c, nil
		}
		if best == nil || expiryRank(c) > expiryRank(best) {
			best = c
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%s에 유효한 %q 쿠키가 없습니다 — 먼저 `agentlayer browser cookies import %s`로 가져오거나 "+
			"`cookies list %s`로 이름을 확인하세요", domain, name, domain, domain)
	}
	return best, nil
}

func expiryRank(c *proto.NetworkCookie) float64 {
	if c.Expires <= 0 {
		return 1e18
	}
	return float64(c.Expires)
}

// FormatNetscape는 Netscape cookies.txt 형식(yt-dlp·curl·wget이 읽는 표준)으로 만든다.
// HttpOnly 쿠키는 curl 관례대로 "#HttpOnly_" 접두어를 붙인다(yt-dlp도 인식).
func FormatNetscape(cs []*proto.NetworkCookie) string {
	var sb strings.Builder
	sb.WriteString("# Netscape HTTP Cookie File\n# https://curl.se/docs/http-cookies.html\n# agentlayer browser cookies export\n\n")
	for _, c := range cs {
		domain := c.Domain
		if c.HTTPOnly {
			domain = "#HttpOnly_" + domain
		}
		includeSub := "FALSE"
		if strings.HasPrefix(c.Domain, ".") {
			includeSub = "TRUE"
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}
		var exp int64
		if c.Expires > 0 {
			exp = int64(c.Expires)
		}
		fmt.Fprintf(&sb, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", domain, includeSub, path, secure, exp, c.Name, c.Value)
	}
	return sb.String()
}

type cookieJSON struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  int64  `json:"expires"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
}

// FormatCookieJSON은 이름·값·호스트·경로·만료(epoch초, 세션은 0)·secure·httpOnly 배열.
func FormatCookieJSON(cs []*proto.NetworkCookie) string {
	rows := make([]cookieJSON, 0, len(cs))
	for _, c := range cs {
		var exp int64
		if c.Expires > 0 {
			exp = int64(c.Expires)
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		rows = append(rows, cookieJSON{c.Name, c.Value, c.Domain, path, exp, c.Secure, c.HTTPOnly})
	}
	b, _ := json.MarshalIndent(rows, "", "  ")
	return string(b) + "\n"
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// writeSecretFile은 0600으로 쓴다(디렉터리 생성, 기존 파일은 덮어쓰되 권한도 0600으로 맞춤).
func writeSecretFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func envQuote(v string) string {
	if strings.ContainsAny(v, " \t#\"'\\$`\n") {
		return strconv.Quote(v)
	}
	return v
}

// updateEnvFile은 .env에서 KEY= 줄만 바꾸고(없으면 끝에 추가) 나머지 줄은 그대로 둔다. 결과는 0600.
func updateEnvFile(path, key, value string) error {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := key + "=" + envQuote(value)
	var lines []string
	if len(raw) > 0 {
		lines = strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	}
	replaced := false
	for i, l := range lines {
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "export "))
		if strings.HasPrefix(t, key+"=") {
			lines[i] = line
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, line)
	}
	return writeSecretFile(path, strings.Join(lines, "\n")+"\n")
}

func expiryLabel(c *proto.NetworkCookie, now time.Time) string {
	if c.Expires <= 0 {
		return "세션 쿠키"
	}
	return "만료 " + time.Unix(int64(c.Expires), 0).Local().Format("2006-01-02")
}

// ExportCookies는 전용 프로필에서 읽어 파일에 쓴다. out에는 값이 아닌 요약만 찍는다.
func ExportCookies(b *rod.Browser, o ExportOptions, now time.Time, out io.Writer) error {
	if o.Domain == "" {
		return fmt.Errorf("도메인을 지정하세요 (예: agentlayer browser cookies export claude.ai sessionKey --to ~/x/key.txt)")
	}
	if (o.To == "") == (o.EnvFile == "") {
		return fmt.Errorf("--to <파일> 또는 --env <.env파일> <KEY> 중 하나를 지정하세요 (값은 화면에 출력하지 않습니다)")
	}
	if o.EnvFile != "" && (o.Name == "" || o.EnvKey == "") {
		return fmt.Errorf("--env는 쿠키 이름과 KEY가 필요합니다 (예: cookies export claude.ai sessionKey --env ~/bot/.env CLAUDE_SESSION_KEY)")
	}
	format := o.Format
	if format == "" {
		format = FormatNetscapeF
		if o.Name != "" {
			format = FormatValue
		}
	}
	if format == FormatValue && o.Name == "" {
		return fmt.Errorf("value 형식은 쿠키 이름이 필요합니다 (cookies list %s 로 이름 확인)", o.Domain)
	}
	res, err := proto.StorageGetCookies{}.Call(b)
	if err != nil {
		return fmt.Errorf("쿠키 조회 실패: %w", err)
	}
	var cs []*proto.NetworkCookie
	if o.Name != "" {
		c, err := pickCookie(res.Cookies, o.Domain, o.Name, now)
		if err != nil {
			return err
		}
		cs = []*proto.NetworkCookie{c}
	} else {
		cs = exportableCookies(res.Cookies, o.Domain, now)
		if len(cs) == 0 {
			return fmt.Errorf("%s에 유효한 쿠키가 없습니다 — 먼저 `agentlayer browser cookies import %s`", o.Domain, o.Domain)
		}
	}
	if o.EnvFile != "" {
		path := expandHome(o.EnvFile)
		if err := updateEnvFile(path, o.EnvKey, cs[0].Value); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s %s (%s) → %s 의 %s 갱신, %s. 값은 출력하지 않습니다.\n",
			o.Domain, cs[0].Name, cs[0].Domain, path, o.EnvKey, expiryLabel(cs[0], now))
		return nil
	}
	path := expandHome(o.To)
	var content string
	switch format {
	case FormatValue:
		content = cs[0].Value + "\n"
	case FormatNetscapeF:
		content = FormatNetscape(cs)
	case FormatJSONF:
		content = FormatCookieJSON(cs)
	default:
		return fmt.Errorf("알 수 없는 형식 %q (value|netscape|json)", format)
	}
	if err := writeSecretFile(path, content); err != nil {
		return err
	}
	if o.Name != "" {
		fmt.Fprintf(out, "%s %s (%s) → %s (%s, 0600), %s. 값은 출력하지 않습니다.\n",
			o.Domain, cs[0].Name, cs[0].Domain, path, format, expiryLabel(cs[0], now))
	} else {
		fmt.Fprintf(out, "%s 쿠키 %d개 → %s (%s, 0600). 값은 출력하지 않습니다.\n", o.Domain, len(cs), path, format)
	}
	return nil
}
