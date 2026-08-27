package cli

import (
	"regexp"
	"runtime"
	"strings"
)

// VersionInfo는 빌드 시 주입되는 버전 정보다.
// goreleaser 기본 ldflags가 main.version/commit/date를 채우고 main.go가 넘긴다.
// 로컬 `go build`에서는 비어 있어 debug.ReadBuildInfo 폴백을 쓴다.
type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

// FormatVersion은 `agentlayer version` 출력을 만든다. 두 줄:
//
//	agentlayer v1.1.0 (commit 340e7fd, 2026-08-27)
//	go1.24.0 darwin/arm64
func FormatVersion(v VersionInfo) string {
	ver := strings.TrimSpace(v.Version)
	if ver == "" {
		ver = "dev"
	}
	if ver != "dev" && !strings.HasPrefix(ver, "v") {
		ver = "v" + ver
	}

	var parts []string
	if c := shortCommit(v.Commit); c != "" {
		parts = append(parts, "commit "+c)
	}
	if d := shortDate(v.Date); d != "" {
		parts = append(parts, d)
	}

	head := "agentlayer " + ver
	if len(parts) > 0 {
		head += " (" + strings.Join(parts, ", ") + ")"
	}
	return head + "\n" + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
}

// shortCommit은 커밋 해시를 7자로 줄인다. dirty 표시는 보존한다.
func shortCommit(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return ""
	}
	suffix := ""
	if strings.HasSuffix(c, "+dirty") {
		c, suffix = strings.TrimSuffix(c, "+dirty"), "+dirty"
	}
	if len(c) > 7 {
		c = c[:7]
	}
	return c + suffix
}

// shortDate는 RFC3339 타임스탬프에서 날짜만 남긴다.
func shortDate(d string) string {
	d = strings.TrimSpace(d)
	if len(d) >= 10 && d[4] == '-' && d[7] == '-' {
		return d[:10]
	}
	return d
}

// pseudoVersionRe는 go가 태그 없는 커밋에 붙이는 유사버전을 잡는다.
// 예: v1.0.1-0.20260827055951-9d76f9ff8e0a
var pseudoVersionRe = regexp.MustCompile(`[-.]\d{14}-[0-9a-f]{12}`)

// IsReleaseVersion은 사람이 보여줄 만한 릴리즈 태그인지 판단한다.
// 유사버전·(devel)·dirty 표시는 버전으로 쓰지 않고 "dev"로 떨어뜨린다.
func IsReleaseVersion(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || v == "(devel)" {
		return false
	}
	if strings.Contains(v, "+dirty") || pseudoVersionRe.MatchString(v) {
		return false
	}
	return true
}
