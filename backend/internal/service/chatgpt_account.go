package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const PlatformChatGPT = "chatgpt"
const chatGPTClientID = "app_2SKx67EdpoN0G6j64rFvigXD"

// CredentialState is a secret-free projection. Unknown expiry is never a claim
// that the upstream accepted a token; refreshability only means RT is present.
type CredentialState struct {
	Access     string `json:"access"`
	Refresh    string `json:"refresh"`
	IssuedAt   *int64 `json:"issued_at,omitempty"`
	ExpiresAt  *int64 `json:"expires_at,omitempty"`
	CanRefresh bool   `json:"can_refresh"`
}

func AccountCredentialState(a *Account, now time.Time) *CredentialState {
	if a == nil || (a.Type != AccountTypeOAuth && a.Type != AccountTypeSetupToken) {
		return nil
	}
	state := &CredentialState{Access: "missing", Refresh: "missing"}
	if a.IsCredentialShadow() {
		state.Access = "inherited"
		state.Refresh = "inherited"
		return state
	}
	if strings.TrimSpace(a.GetCredential("refresh_token")) != "" {
		state.Refresh = "present"
		state.CanRefresh = a.Type == AccountTypeOAuth
		if a.Platform == PlatformChatGPT && a.Extra["chatgpt_refresh_invalid"] == true {
			state.Refresh = "invalid"
			state.CanRefresh = false
		}
	}
	token := strings.TrimSpace(a.GetCredential("access_token"))
	if token == "" {
		return state
	}
	state.Access = "unknown"
	var claims struct {
		Iat int64 `json:"iat"`
		Exp int64 `json:"exp"`
	}
	parts := strings.Split(token, ".")
	if len(parts) == 3 {
		if payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "=")); err == nil {
			_ = json.Unmarshal(payload, &claims)
		}
	}
	if claims.Iat > 0 {
		state.IssuedAt = &claims.Iat
	}
	expires := a.GetCredentialAsTime("expires_at")
	if claims.Exp > 0 && (expires == nil || claims.Exp < expires.Unix()) {
		t := time.Unix(claims.Exp, 0)
		expires = &t
	}
	if expires != nil {
		ts := expires.Unix()
		state.ExpiresAt = &ts
		switch {
		case !expires.After(now):
			state.Access = "expired"
		case expires.Sub(now) <= 24*time.Hour:
			state.Access = "expiring"
		default:
			state.Access = "valid"
		}
	}
	if a.Platform == PlatformChatGPT && a.Extra["chatgpt_access_invalid"] == true {
		state.Access = "invalid"
	}
	return state
}

type ChatGPTSnapshot struct {
	Email          string `json:"email,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	AccountID      string `json:"account_id,omitempty"`
	Plan           string `json:"plan,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	QuotaRemaining *int   `json:"quota_remaining"`
	QuotaResetAt   string `json:"quota_reset_at,omitempty"`
	Deactivated    bool   `json:"deactivated"`
	SyncedAt       string `json:"synced_at"`
}

type ChatGPTPoolState struct {
	Status string `json:"status"`
	Quota  string `json:"quota"`
}

func ChatGPTAccountPoolState(a *Account, now time.Time) *ChatGPTPoolState {
	if a == nil || a.Platform != PlatformChatGPT {
		return nil
	}
	state := &ChatGPTPoolState{Status: "pending", Quota: "unknown"}
	var snapshot ChatGPTSnapshot
	if raw, ok := a.Extra["chatgpt_snapshot"]; ok {
		data, err := json.Marshal(raw)
		if err == nil && json.Unmarshal(data, &snapshot) == nil && snapshot.SyncedAt != "" {
			state.Status = "normal"
			if snapshot.QuotaRemaining != nil {
				state.Quota = "available"
				if *snapshot.QuotaRemaining <= 0 {
					state.Quota = "exhausted"
					state.Status = "limited"
				}
				if reset, err := time.Parse(time.RFC3339, snapshot.QuotaResetAt); err == nil && !reset.After(now) {
					state.Quota = "unknown"
					state.Status = "pending"
				}
			} else if strings.EqualFold(snapshot.Plan, "pro") || strings.EqualFold(snapshot.Plan, "prolite") {
				state.Quota = "unlimited"
			}
			if snapshot.Deactivated {
				state.Status = "abnormal"
			}
		}
	}
	credentials := AccountCredentialState(a, now)
	if credentials.Access == "expired" || credentials.Access == "invalid" || credentials.Access == "missing" {
		state.Status = "abnormal"
		if credentials.CanRefresh {
			state.Status = "recoverable"
		}
	}
	if a.Status == StatusError {
		state.Status = "abnormal"
	}
	if a.Status != StatusActive && a.Status != StatusError || !a.Schedulable {
		state.Status = "disabled"
	}
	return state
}

// ChatGPTAccountManager is optional on AdminService so existing adapters do not
// need to implement provider-specific maintenance.
type ChatGPTAccountManager interface {
	MaintainChatGPTAccount(context.Context, int64, string) (*Account, error)
	ListChatGPTAccountPool(context.Context, int64) ([]Account, error)
}

func (s *adminServiceImpl) ListChatGPTAccountPool(ctx context.Context, groupID int64) ([]Account, error) {
	if groupID > 0 {
		if err := s.ValidateAccountGroupBindings(ctx, []int64{groupID}); err != nil {
			return nil, err
		}
	}
	return s.accountRepo.ListAllWithFilters(ctx, PlatformChatGPT, AccountTypeOAuth, "", "", groupID, "")
}

type ChatGPTAccountStateRepository interface {
	UpdateChatGPTAccountState(context.Context, *Account, map[string]any, map[string]any) (bool, error)
	AcquireChatGPTMaintenance(context.Context, int64, string) (bool, error)
	ReleaseChatGPTMaintenance(context.Context, int64, string) error
}

var chatGPTMaintenanceSlots = make(chan struct{}, 10)

// Use the existing refresh service's lifecycle and cancellation budget. Quota
// checks are separate from RT exchanges and have a fifteen-minute retry window.
func (s *TokenRefreshService) maintainChatGPTPool(ctx context.Context) {
	if s.chatgptManager == nil || ctx.Err() != nil {
		return
	}
	accounts, err := s.chatgptManager.ListChatGPTAccountPool(ctx, 0)
	if err != nil {
		slog.Warn("chatgpt.pool_list_failed")
		return
	}
	window := time.Hour
	if s.cfg != nil && s.cfg.RefreshBeforeExpiryHours > 0 {
		window = time.Duration(s.cfg.RefreshBeforeExpiryHours) * time.Hour
	}
	g, taskCtx := errgroup.WithContext(ctx)
	g.SetLimit(5)
	for i := range accounts {
		a := accounts[i]
		if taskCtx.Err() != nil {
			break
		}
		if a.Status != StatusActive || !a.Schedulable || a.Extra["chatgpt_auto_maintain"] == false {
			continue
		}
		g.Go(func() error {
			state := AccountCredentialState(&a, time.Now())
			if state.CanRefresh && state.ExpiresAt != nil && time.Until(time.Unix(*state.ExpiresAt, 0)) <= window {
				if _, err := s.chatgptManager.MaintainChatGPTAccount(taskCtx, a.ID, "refresh"); err != nil {
					slog.Warn("chatgpt.auto_refresh_failed", "account_id", a.ID)
					return nil
				}
			}
			last, _ := a.Extra["chatgpt_last_sync_at"].(string)
			if timestamp, err := time.Parse(time.RFC3339, last); err == nil && time.Since(timestamp) < 15*time.Minute {
				return nil
			}
			if _, err := s.chatgptManager.MaintainChatGPTAccount(taskCtx, a.ID, "sync"); err != nil {
				slog.Warn("chatgpt.auto_sync_failed", "account_id", a.ID)
			}
			return nil
		})
	}
	_ = g.Wait()
}

func (s *adminServiceImpl) MaintainChatGPTAccount(ctx context.Context, id int64, action string) (*Account, error) {
	if action != "sync" && action != "refresh" {
		return nil, infraerrors.BadRequest("INVALID_ACTION", "unsupported account action")
	}
	select {
	case chatGPTMaintenanceSlots <- struct{}{}:
		defer func() { <-chatGPTMaintenanceSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// Once a refresh starts, keep its bounded rotation/persistence lifecycle alive
	// if the browser disconnects: the provider may consume the previous RT.
	if action == "refresh" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()
	}
	a, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.Platform != PlatformChatGPT || a.Type != AccountTypeOAuth {
		return nil, infraerrors.BadRequest("INVALID_CHATGPT_ACCOUNT", "a ChatGPT OAuth account is required")
	}
	repo, ok := s.accountRepo.(ChatGPTAccountStateRepository)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("CHATGPT_UNAVAILABLE", "account maintenance is unavailable")
	}
	lease := uuid.NewString()
	acquired, err := repo.AcquireChatGPTMaintenance(ctx, id, lease)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, infraerrors.Conflict("ACCOUNT_BUSY", "account maintenance is already running; retry later")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = repo.ReleaseChatGPTMaintenance(cleanup, id, lease)
	}()
	a, err = s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if a.ProxyID != nil {
		proxy, proxyErr := s.proxyRepo.GetByID(ctx, *a.ProxyID)
		if proxyErr != nil {
			return nil, proxyErr
		}
		if proxy == nil || !proxy.IsActive() || proxy.IsExpired(time.Now()) {
			return nil, infraerrors.BadRequest("PROXY_UNAVAILABLE", "the selected proxy is unavailable")
		}
		proxyURL = proxy.URL()
	}
	client, err := httpclient.GetClient(httpclient.Options{ProxyURL: proxyURL, Timeout: 45 * time.Second, ResponseHeaderTimeout: 30 * time.Second})
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_PROXY", "cannot configure the selected proxy")
	}
	// Do not follow redirects with upstream credentials.
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	remote := chatGPTAccountClient{client: &copyClient, baseURL: "https://chatgpt.com", tokenURL: "https://auth.openai.com/oauth/token", deviceID: uuid.NewString()}
	creds, extra := map[string]any{}, map[string]any{}
	if action == "refresh" {
		creds, err = remote.refresh(ctx, a)
		if err == nil {
			extra["chatgpt_access_invalid"] = false
			extra["chatgpt_refresh_invalid"] = false
			extra["chatgpt_last_token_refresh_at"] = time.Now().UTC().Format(time.RFC3339)
		}
	} else {
		var snapshot *ChatGPTSnapshot
		snapshot, err = remote.sync(ctx, a.GetCredential("access_token"))
		if err == nil {
			extra["chatgpt_snapshot"] = snapshot
			extra["chatgpt_access_invalid"] = false
			for key, value := range map[string]string{"email": snapshot.Email, "chatgpt_user_id": snapshot.UserID, "chatgpt_account_id": snapshot.AccountID, "plan_type": snapshot.Plan} {
				if value != "" {
					creds[key] = value
				}
			}
		}
	}
	operationErr := err
	if operationErr != nil {
		var upstream *chatGPTUpstreamError
		if errors.As(operationErr, &upstream) {
			if action == "sync" && upstream.status == http.StatusUnauthorized {
				extra["chatgpt_access_invalid"] = true
			}
			if action == "refresh" && upstream.terminal {
				extra["chatgpt_refresh_invalid"] = true
			}
		}
		extra["chatgpt_last_"+action+"_error"] = operationErr.Error()
	} else {
		extra["chatgpt_last_"+action+"_error"] = ""
	}
	extra["chatgpt_last_"+action+"_at"] = time.Now().UTC().Format(time.RFC3339)
	updated, saveErr := repo.UpdateChatGPTAccountState(ctx, a, creds, extra)
	if saveErr != nil {
		return nil, saveErr
	}
	if !updated {
		return nil, infraerrors.Conflict("ACCOUNT_CHANGED", "credentials or proxy changed during maintenance; retry")
	}
	if operationErr != nil {
		return nil, infraerrors.BadRequest("CHATGPT_UPSTREAM_ERROR", operationErr.Error())
	}
	return s.accountRepo.GetByID(ctx, id)
}

type chatGPTUpstreamError struct {
	status   int
	code     string
	terminal bool
}

func (e *chatGPTUpstreamError) Error() string {
	if e.code != "" {
		return fmt.Sprintf("ChatGPT HTTP %d (%s)", e.status, e.code)
	}
	return fmt.Sprintf("ChatGPT HTTP %d", e.status)
}

type chatGPTAccountClient struct {
	client            *http.Client
	baseURL, tokenURL string
	deviceID          string
}

func (c *chatGPTAccountClient) request(ctx context.Context, method, endpoint, token, contentType string, body io.Reader, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("cannot build upstream request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/145.0.0.0 Safari/537.36")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Origin", c.baseURL)
		req.Header.Set("Oai-Device-Id", c.deviceID)
		req.Header.Set("Oai-Language", "en-US")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Referer", c.baseURL+"/")
		req.Header.Set("X-OpenAI-Target-Path", req.URL.Path)
		req.Header.Set("X-OpenAI-Target-Route", req.URL.Path)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("ChatGPT network request failed; check the account proxy")
	}
	defer func() { _ = resp.Body.Close() }()
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(data) > limit {
		return errors.New("invalid or oversized ChatGPT response")
	}
	if resp.StatusCode != http.StatusOK {
		e := &chatGPTUpstreamError{status: resp.StatusCode}
		var payload map[string]json.RawMessage
		if json.Unmarshal(data, &payload) == nil {
			var code string
			if json.Unmarshal(payload["error"], &code) != nil {
				var nested struct {
					Code string `json:"code"`
				}
				_ = json.Unmarshal(payload["error"], &nested)
				code = nested.Code
			}
			switch code {
			case "invalid_grant", "invalid_refresh_token", "refresh_token_reused", "refresh_token_invalidated", "token_expired", "app_session_terminated":
				e.code = code
				e.terminal = resp.StatusCode == 400 || resp.StatusCode == 401
			}
		}
		return e
	}
	if json.Unmarshal(data, target) != nil {
		return errors.New("ChatGPT returned an invalid JSON response")
	}
	return nil
}

func (c *chatGPTAccountClient) refresh(ctx context.Context, a *Account) (map[string]any, error) {
	rt := strings.TrimSpace(a.GetCredential("refresh_token"))
	if rt == "" {
		return nil, errors.New("refresh token is missing; import new credentials")
	}
	clientID := strings.TrimSpace(a.GetCredential("client_id"))
	if clientID == "" {
		clientID = chatGPTClientID
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {rt}, "client_id": {clientID}}
	var result struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		ID      string `json:"id_token"`
		Expires int64  `json:"expires_in"`
	}
	if err := c.request(ctx, http.MethodPost, c.tokenURL, "", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), &result); err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.Access) == "" {
		return nil, errors.New("ChatGPT refresh response has no access token")
	}
	creds := map[string]any{"access_token": result.Access, "client_id": clientID}
	if result.Refresh != "" {
		creds["refresh_token"] = result.Refresh
	}
	if result.ID != "" {
		creds["id_token"] = result.ID
	}
	// Explicitly clear a previous token's expiry when the provider omits it.
	creds["expires_at"] = ""
	if result.Expires > 0 {
		creds["expires_at"] = time.Now().Add(time.Duration(result.Expires) * time.Second).UTC().Format(time.RFC3339)
	}
	return creds, nil
}

func (c *chatGPTAccountClient) sync(ctx context.Context, token string) (*ChatGPTSnapshot, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("access token is missing")
	}
	var me struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	var init struct {
		Limits []struct {
			Feature   string          `json:"feature_name"`
			Remaining *int            `json:"remaining"`
			Reset     json.RawMessage `json:"reset_after"`
		} `json:"limits_progress"`
	}
	var check struct {
		Accounts map[string]struct {
			Account struct {
				ID          string `json:"account_id"`
				Plan        string `json:"plan_type"`
				Deactivated bool   `json:"is_deactivated"`
			} `json:"account"`
		} `json:"accounts"`
	}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return c.request(gctx, http.MethodGet, c.baseURL+"/backend-api/me", token, "", nil, &me) })
	g.Go(func() error {
		return c.request(gctx, http.MethodPost, c.baseURL+"/backend-api/conversation/init", token, "application/json", bytes.NewBufferString(`{"gizmo_id":null,"requested_default_model":null,"conversation_id":null,"timezone_offset_min":-480}`), &init)
	})
	g.Go(func() error {
		return c.request(gctx, http.MethodGet, c.baseURL+"/backend-api/accounts/check/v4-2023-04-27?timezone_offset_min=-480", token, "", nil, &check)
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	account, exists := check.Accounts["default"]
	if me.ID == "" || !exists {
		return nil, errors.New("incomplete ChatGPT account response")
	}
	snapshot := &ChatGPTSnapshot{Email: me.Email, UserID: me.ID, DisplayName: me.Name, AccountID: account.Account.ID, Plan: account.Account.Plan, Deactivated: account.Account.Deactivated, SyncedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, limit := range init.Limits {
		if limit.Feature != "image_gen" {
			continue
		}
		snapshot.QuotaRemaining = limit.Remaining
		if json.Unmarshal(limit.Reset, &snapshot.QuotaResetAt) != nil {
			var seconds float64
			if json.Unmarshal(limit.Reset, &seconds) == nil && seconds > 0 {
				snapshot.QuotaResetAt = time.Now().Add(time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339)
			}
		}
		break
	}
	return snapshot, nil
}
