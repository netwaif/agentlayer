package cli

import "testing"

func TestFormatVersionRelease(t *testing.T) {
	got := FormatVersion(VersionInfo{
		Version: "1.1.0",
		Commit:  "340e7fdabc1234567890",
		Date:    "2026-08-27T10:11:12Z",
	})
	want := "agentlayer v1.1.0 (commit 340e7fd, 2026-08-27)"
	if first := firstLine(got); first != want {
		t.Fatalf("첫 줄\n got: %q\nwant: %q", first, want)
	}
	if second := secondLine(got); second == "" {
		t.Fatalf("둘째 줄에 go 런타임 정보가 있어야 한다: %q", got)
	}
}

func TestFormatVersionDevFallback(t *testing.T) {
	got := firstLine(FormatVersion(VersionInfo{}))
	want := "agentlayer dev"
	if got != want {
		t.Fatalf("빈 정보면 dev\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatVersionKeepsVPrefixOnce(t *testing.T) {
	got := firstLine(FormatVersion(VersionInfo{Version: "v1.1.0"}))
	want := "agentlayer v1.1.0"
	if got != want {
		t.Fatalf("v 중복 금지\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatVersionCommitOnly(t *testing.T) {
	got := firstLine(FormatVersion(VersionInfo{Version: "dev", Commit: "340e7fdabc"}))
	want := "agentlayer dev (commit 340e7fd)"
	if got != want {
		t.Fatalf("커밋만 있을 때\n got: %q\nwant: %q", got, want)
	}
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

func secondLine(s string) string {
	rest := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			rest = s[i+1:]
			break
		}
	}
	return firstLine(rest)
}

func TestIsReleaseVersion(t *testing.T) {
	cases := map[string]bool{
		"v1.1.0":                               true,
		"1.1.0":                                true,
		"v1.2.0-rc1":                           true,
		"":                                     false,
		"(devel)":                              false,
		"v1.0.1-0.20260827055951-9d76f9ff8e0a": false, // go가 만든 유사버전
		"v1.0.1-0.20260827055951-9d76f9ff8e0a+dirty": false,
	}
	for in, want := range cases {
		if got := IsReleaseVersion(in); got != want {
			t.Errorf("IsReleaseVersion(%q) = %v, want %v", in, got, want)
		}
	}
}
