package getui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lzyats/core-push-go/pkg/push"
)

// Provider implements GeTui RestAPI v2.
//
// Token:
//
//	POST BaseUrl/auth with JSON body {sign,timestamp,appkey}
//
// Push (CID single):
//
//	POST BaseUrl/push/single/cid with header token: <token>
//
// References: GeTui doc RestAPI v2 token + push single cid.citeturn4search0turn3view0
type Provider struct {
	cfg        push.PushSettings
	httpClient *http.Client

	mu       sync.Mutex
	token    string
	expireAt time.Time
}

func New(cfg push.PushSettings) *Provider {
	c := cfg
	if c.BaseURL == "" {
		c.BaseURL = "https://restapi.getui.com/v2"
	}
	if c.TTLMillis <= 0 {
		c.TTLMillis = 2 * 60 * 60 * 1000
	}
	return &Provider{
		cfg:        c,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *Provider) Type() string { return "getui" }

func (p *Provider) Push(ctx context.Context, msg push.Message) (push.Result, error) {
	if p.cfg.AppID == "" || p.cfg.AppKey == "" || p.cfg.MasterSecret == "" {
		return push.Result{OK: false, Provider: p.Type(), At: time.Now(), Error: "missing appId/appKey/masterSecret"}, push.ErrNotConfigured
	}
	if len(msg.Targets) == 0 {
		return push.Result{OK: false, Provider: p.Type(), At: time.Now(), Error: "missing targets"}, push.ErrInvalidArgument
	}

	tok, err := p.getToken(ctx)
	if err != nil {
		return push.Result{OK: false, Provider: p.Type(), At: time.Now(), Error: err.Error()}, err
	}

	// Build request payload per docs.
	reqBody := map[string]any{
		"request_id": fmt.Sprintf("%d", time.Now().UnixNano()),
		"settings": map[string]any{
			"ttl": p.cfg.TTLMillis,
		},
		"audience": map[string]any{
			"cid": msg.Targets,
		},
		"push_message": map[string]any{
			"notification": map[string]any{
				"title":      msg.Title,
				"body":       msg.Body,
				"click_type": "none",
			},
		},
	}
	if len(msg.Data) > 0 {
		// Put as transmission payload (GeTui supports transmission/message in other fields;
		// here we attach to notification as an extension field for downstream usage.
		reqBody["push_message"].(map[string]any)["transmission"] = msg.Data
	}

	b, _ := json.Marshal(reqBody)
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/" + p.cfg.AppID + "/push/single/cid"
	hreq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	hreq.Header.Set("Content-Type", "application/json;charset=utf-8")
	hreq.Header.Set("token", tok)

	resp, err := p.httpClient.Do(hreq)
	if err != nil {
		return push.Result{OK: false, Provider: p.Type(), At: time.Now(), Error: err.Error()}, err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("getui: http %d: %s", resp.StatusCode, bodyStr)
		return push.Result{OK: false, Provider: p.Type(), At: time.Now(), Body: bodyStr, Error: err.Error()}, err
	}

	// Parse doc return: {code,msg,data:{taskid:{cid:status}}}
	var r struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(bodyBytes, &r)
	ok := r.Code == 0
	if !ok {
		err = fmt.Errorf("getui: code=%d msg=%s", r.Code, r.Msg)
	}
	return push.Result{OK: ok, Provider: p.Type(), At: time.Now(), Body: bodyStr, Error: errString(err)}, err
}

func (p *Provider) getToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.token != "" && time.Now().Before(p.expireAt.Add(-2*time.Minute)) {
		t := p.token
		p.mu.Unlock()
		return t, nil
	}
	p.mu.Unlock()

	// Refresh token
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	sign := sha256Hex(p.cfg.AppKey + timestamp + p.cfg.MasterSecret)
	payload := map[string]string{
		"sign":      sign,
		"timestamp": timestamp,
		"appkey":    p.cfg.AppKey,
	}
	b, _ := json.Marshal(payload)
	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/" + p.cfg.AppID + "/auth"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getui auth: http %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var r struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ExpireTime string `json:"expire_time"`
			Token      string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &r); err != nil {
		return "", err
	}
	if r.Code != 0 || r.Data.Token == "" {
		if r.Code == 0 {
			err = errors.New("getui auth: empty token")
		} else {
			err = fmt.Errorf("getui auth: code=%d msg=%s", r.Code, r.Msg)
		}
		return "", err
	}

	exp := time.Now().Add(23 * time.Hour)
	if r.Data.ExpireTime != "" {
		if ms, e := parseInt64(r.Data.ExpireTime); e == nil && ms > 0 {
			exp = time.UnixMilli(ms)
		}
	}

	p.mu.Lock()
	p.token = r.Data.Token
	p.expireAt = exp
	p.mu.Unlock()

	return r.Data.Token, nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func parseInt64(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty")
	}
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, errors.New("not number")
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}
