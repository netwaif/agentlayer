package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// `agentlayer usage setup claude [--undo]` — Claude Code 설정(settings.json)의 statusLine을 tap으로 감싸거나 되돌린다.
// 원래 상태줄 명령은 `sh -c '<원래 명령>'`로 그대로 이어 붙여(파이프·따옴표가 있는 한 줄 명령도) 똑같이 보인다.
// 고치기 전에 settings.json을 settings.json.bak-agentlayer-<시각>으로 남기고, 다른 칸은 순서·내용을 건드리지 않는다.

const tapMarker = " usage tap claude --"

// shellQuote는 sh 작은따옴표 인용.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }

// shellUnquote는 shellQuote의 반대(우리가 만든 꼴만).
func shellUnquote(s string) (string, bool) {
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return "", false
	}
	return strings.ReplaceAll(s[1:len(s)-1], `'"'"'`, "'"), true
}

// WrapStatusCommand는 원래 명령을 tap으로 감싼 명령(원래가 없으면 tap만 — 상태줄에는 아무것도 안 나온다).
func WrapStatusCommand(bin, orig string) string {
	w := shellQuote(bin) + tapMarker
	if strings.TrimSpace(orig) != "" {
		w += " sh -c " + shellQuote(orig)
	}
	return w
}

// UnwrapStatusCommand는 감싼 명령에서 원래 명령을 꺼낸다. 감싼 것이 아니면 ok=false.
func UnwrapStatusCommand(cmd string) (orig string, ok bool) {
	i := strings.Index(cmd, tapMarker)
	if i < 0 {
		return "", false
	}
	rest := strings.TrimSpace(cmd[i+len(tapMarker):])
	if rest == "" {
		return "", true
	}
	if !strings.HasPrefix(rest, "sh -c ") {
		return "", false
	}
	return shellUnquote(strings.TrimSpace(rest[len("sh -c "):]))
}

// 최상위 키 순서를 지키는 JSON 객체(값은 원문 그대로)
type orderedObj struct {
	keys []string
	vals map[string]json.RawMessage
}

func parseOrdered(b []byte) (*orderedObj, error) {
	o := &orderedObj{vals: map[string]json.RawMessage{}}
	if len(bytes.TrimSpace(b)) == 0 {
		return o, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("settings.json 최상위가 객체가 아님")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		if _, dup := o.vals[k]; !dup {
			o.keys = append(o.keys, k)
		}
		o.vals[k] = v
	}
	return o, nil
}

func (o *orderedObj) set(k string, v json.RawMessage) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *orderedObj) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// compact는 한 줄 JSON(바깥 marshal이 들여쓴다).
func (o *orderedObj) compact() json.RawMessage {
	var b bytes.Buffer
	b.WriteString("{")
	for i, k := range o.keys {
		if i > 0 {
			b.WriteString(",")
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteString(":")
		b.Write(o.vals[k])
	}
	b.WriteString("}")
	return b.Bytes()
}

func (o *orderedObj) marshal() []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, k := range o.keys {
		kb, _ := json.Marshal(k)
		var vb bytes.Buffer
		if json.Indent(&vb, o.vals[k], "  ", "  ") != nil {
			vb.Reset()
			vb.Write(o.vals[k])
		}
		fmt.Fprintf(&b, "  %s: %s", kb, vb.Bytes())
		if i < len(o.keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

// SetupResult는 무엇을 했는지(출력용).
type SetupResult struct {
	Settings string // 고친 파일
	Backup   string // 남긴 사본(안 고쳤으면 "")
	Before   string // 고치기 전 statusLine.command
	After    string // 고친 뒤
	Changed  bool
}

// SetupClaude는 configDir/settings.json의 statusLine을 tap으로 감싼다(undo면 되돌린다). bin은 agentlayer 실행 파일 절대 경로.
func SetupClaude(configDir, bin string, undo bool, now time.Time) (SetupResult, error) {
	path := filepath.Join(configDir, "settings.json")
	// 심볼릭 링크면 실제 파일을 고친다
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	res := SetupResult{Settings: path}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return res, err
	}
	obj, err := parseOrdered(raw)
	if err != nil {
		return res, fmt.Errorf("%s 읽기: %w", path, err)
	}
	// statusLine 안의 칸도 순서·원문을 지킨다(command만 바꾼다)
	var sl *orderedObj
	if v, ok := obj.vals["statusLine"]; ok {
		if o, err := parseOrdered(v); err == nil {
			sl = o
		}
	}
	cur := ""
	if sl != nil {
		_ = json.Unmarshal(sl.vals["command"], &cur)
	}
	res.Before = cur
	_, wrapped := UnwrapStatusCommand(cur)
	switch {
	case !undo && wrapped:
		res.After = cur
		return res, nil // 이미 감쌈
	case undo && !wrapped:
		res.After = cur
		return res, nil // 감싼 적 없음
	}
	str := func(v string) json.RawMessage { b, _ := json.Marshal(v); return b }
	if !undo {
		if sl == nil {
			sl = &orderedObj{vals: map[string]json.RawMessage{}}
		}
		sl.set("type", str("command"))
		res.After = WrapStatusCommand(bin, cur)
		sl.set("command", str(res.After))
	} else {
		orig, _ := UnwrapStatusCommand(cur)
		if orig == "" {
			sl = nil // 우리가 새로 넣었던 것 — 지운다
		} else {
			sl.set("command", str(orig))
			res.After = orig
		}
	}
	if sl == nil {
		obj.del("statusLine")
		res.After = ""
	} else {
		obj.set("statusLine", sl.compact())
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return res, err
	}
	if len(raw) > 0 {
		// 같은 초에 두 번 고쳐도 앞 사본을 덮지 않는다
		res.Backup = path + ".bak-agentlayer-" + now.Format("20060102-150405")
		for i := 2; ; i++ {
			if _, err := os.Stat(res.Backup); os.IsNotExist(err) {
				break
			}
			res.Backup = fmt.Sprintf("%s.bak-agentlayer-%s-%d", path, now.Format("20060102-150405"), i)
		}
		if err := os.WriteFile(res.Backup, raw, 0o600); err != nil {
			return res, err
		}
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := writeFileAtomic(path, obj.marshal(), mode); err != nil {
		return res, err
	}
	res.Changed = true
	return res, nil
}
