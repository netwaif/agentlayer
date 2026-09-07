#!/usr/bin/env bash
# agentlayer 설치 — 최신(또는 AGENTLAYER_VERSION) 릴리즈 tar.gz를 받아 ~/.local/bin/agentlayer 에 놓는다.
# 리눅스/WSL2 기본 설치 경로. 맥에서도 brew 없이 쓸 수 있다.
#   curl -fsSL https://raw.githubusercontent.com/netwaif/agentlayer/main/install.sh | bash
set -euo pipefail
REPO="netwaif/agentlayer"
BIN_DIR="${AGENTLAYER_BIN_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')   # darwin | linux
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "지원하지 않는 아키텍처: $arch" >&2; exit 1 ;;
esac
case "$os" in darwin|linux) ;; *) echo "지원하지 않는 OS: $os (WSL2 안의 리눅스에서 실행하세요)" >&2; exit 1 ;; esac

ver="${AGENTLAYER_VERSION:-}"
if [ -z "$ver" ]; then
  # API 대신 releases/latest 리다이렉트로 태그를 알아낸다(무인증 API 제한 회피)
  ver=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed -E 's#.*/tag/##')
fi
ver="${ver#v}"
url="https://github.com/$REPO/releases/download/v$ver/agentlayer_${ver}_${os}_${arch}.tar.gz"
echo "agentlayer v$ver ($os/$arch) → $BIN_DIR/agentlayer"
if [ "${AGENTLAYER_DRY_RUN:-}" = "1" ]; then echo "dry-run: $url"; exit 0; fi

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/a.tgz"
tar -xzf "$tmp/a.tgz" -C "$tmp" agentlayer
mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/agentlayer" "$BIN_DIR/agentlayer"
echo "설치 완료: $("$BIN_DIR/agentlayer" version 2>/dev/null | head -1 || echo "$BIN_DIR/agentlayer")"
case ":$PATH:" in *":$BIN_DIR:"*) ;; *) echo "PATH에 $BIN_DIR 을 추가하세요: export PATH=\"$BIN_DIR:\$PATH\"" ;; esac
echo "다음: agentlayer init"
