package comet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPSender delivers packets to comet via HTTP.
// cometAddr is taken from Redis route value, e.g. "10.0.0.12:7001" or "http://10.0.0.12:7001".
type HTTPSender struct {
	Client      *http.Client
	PushPath    string // single push endpoint, e.g. "/internal/push"
	PushBatchPath string // batch push endpoint, e.g. "/internal/push/batch"
}

type pushReq struct {
	UID        int64  `json:"uid"`
	PacketJSON string `json:"packet_json"`
}

type batchReq struct {
	Items []pushReq `json:"items"`
}

func NewHTTPSender(timeout time.Duration, pushPath string) *HTTPSender {
	return &HTTPSender{
		Client:        &http.Client{Timeout: timeout},
		PushPath:      pushPath,
		PushBatchPath: pushPath + "/batch",
	}
}

func normalizeAddr(cometAddr string) string {
	addr := cometAddr
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	return strings.TrimRight(addr, "/")
}

func (s *HTTPSender) SendToComet(ctx context.Context, cometAddr string, uid int64, packetJSON string) error {
	if cometAddr == "" {
		return fmt.Errorf("empty cometAddr")
	}
	url := normalizeAddr(cometAddr) + s.PushPath
	body, _ := json.Marshal(pushReq{UID: uid, PacketJSON: packetJSON})
	return s.post(ctx, url, body)
}

// SendBatch sends a batch to one comet node in a single request.
func (s *HTTPSender) SendBatch(ctx context.Context, cometAddr string, items []pushReq) error {
	if cometAddr == "" {
		return fmt.Errorf("empty cometAddr")
	}
	if len(items) == 0 {
		return nil
	}
	url := normalizeAddr(cometAddr) + s.PushBatchPath
	body, _ := json.Marshal(batchReq{Items: items})
	return s.post(ctx, url, body)
}

func (s *HTTPSender) post(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("comet push status=%d", resp.StatusCode)
	}
	return nil
}
