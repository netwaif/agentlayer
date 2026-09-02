package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// PostFile은 파일 하나를 첨부한 일반 메시지를 웹훅으로 보낸다 (푸시 발생).
// 스크린샷을 폰의 Discord로 받아보는 용도 — SSH 원격에서 브라우저를 못 볼 때.
// 에러 메시지에 웹훅 URL(토큰 포함)을 절대 싣지 않는다.
func (c *Client) PostFile(content, filename string, data []byte) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	payload, err := json.Marshal(map[string]any{"username": "agentlayer", "content": content})
	if err != nil {
		return err
	}
	if err := mw.WriteField("payload_json", string(payload)); err != nil {
		return err
	}
	fw, err := mw.CreateFormFile("files[0]", filename)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.Webhook, &body)
	if err != nil {
		return fmt.Errorf("discord 요청 생성 실패")
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("User-Agent", "agentlayer")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("discord 요청 실패")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("discord 파일 게시 실패 (HTTP %d): %s", resp.StatusCode, raw)
	}
	return nil
}
