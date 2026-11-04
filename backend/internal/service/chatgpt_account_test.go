package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func chatGPTTestJWT(t *testing.T, issued, expires int64) string {
	t.Helper()
	payload, err := json.Marshal(map[string]int64{"iat": issued, "exp": expires})
	require.NoError(t, err)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func TestChatGPTCredentialState(t *testing.T) {
	now := time.Unix(1900000000, 0)
	for _, tt := range []struct {
		name, token, refresh, accessState, refreshState string
		extra                                           map[string]any
	}{
		{"missing", "", "", "missing", "missing", nil},
		{"opaque", "opaque", "rt", "unknown", "present", nil},
		{"expired", chatGPTTestJWT(t, now.Add(-time.Hour).Unix(), now.Add(-time.Minute).Unix()), "rt", "expired", "present", nil},
		{"expiring", chatGPTTestJWT(t, now.Unix(), now.Add(time.Hour).Unix()), "rt", "expiring", "present", nil},
		{"valid", chatGPTTestJWT(t, now.Unix(), now.Add(48*time.Hour).Unix()), "rt", "valid", "present", nil},
		{"rejected", chatGPTTestJWT(t, now.Unix(), now.Add(48*time.Hour).Unix()), "rt", "invalid", "invalid", map[string]any{"chatgpt_access_invalid": true, "chatgpt_refresh_invalid": true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Platform: PlatformChatGPT, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": tt.token, "refresh_token": tt.refresh}, Extra: tt.extra}
			state := AccountCredentialState(a, now)
			require.Equal(t, tt.accessState, state.Access)
			require.Equal(t, tt.refreshState, state.Refresh)
			require.Equal(t, tt.refreshState == "present", state.CanRefresh)
			data, err := json.Marshal(state)
			require.NoError(t, err)
			require.NotContains(t, string(data), "opaque")
		})
	}
	shadow := &Account{Type: AccountTypeOAuth, ParentAccountID: new(int64)}
	require.Equal(t, "inherited", AccountCredentialState(shadow, now).Access)
	require.Nil(t, AccountCredentialState(&Account{Type: AccountTypeAPIKey}, now))
}

func TestChatGPTPoolStatePrecedence(t *testing.T) {
	now := time.Unix(1900000000, 0)
	zero := 0
	a := &Account{Platform: PlatformChatGPT, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "opaque", "refresh_token": "rt"}, Extra: map[string]any{"chatgpt_snapshot": &ChatGPTSnapshot{Plan: "plus", QuotaRemaining: &zero, SyncedAt: now.Format(time.RFC3339)}}}
	require.Equal(t, &ChatGPTPoolState{Status: "limited", Quota: "exhausted"}, ChatGPTAccountPoolState(a, now))
	a.Extra["chatgpt_snapshot"] = &ChatGPTSnapshot{Plan: "pro", SyncedAt: now.Format(time.RFC3339)}
	require.Equal(t, "unlimited", ChatGPTAccountPoolState(a, now).Quota)
	a.Extra["chatgpt_access_invalid"] = true
	require.Equal(t, "recoverable", ChatGPTAccountPoolState(a, now).Status)
	a.Extra["chatgpt_refresh_invalid"] = true
	require.Equal(t, "abnormal", ChatGPTAccountPoolState(a, now).Status)
	a.Status = "inactive"
	require.Equal(t, "disabled", ChatGPTAccountPoolState(a, now).Status)
}

func TestChatGPTSyncQuotaAndUnknown(t *testing.T) {
	for _, tt := range []struct {
		name, init string
		remaining  *int
	}{
		{"observed", `{"limits_progress":[{"feature_name":"image_gen","remaining":3,"reset_after":120}]}`, func() *int { n := 3; return &n }()},
		{"unknown", `{"limits_progress":[]}`, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer at-secret", r.Header.Get("Authorization"))
				require.Equal(t, r.URL.Path, r.Header.Get("X-OpenAI-Target-Path"))
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/backend-api/me":
					_, _ = w.Write([]byte(`{"id":"user1","email":"test@example.com","name":"Demo"}`))
				case "/backend-api/conversation/init":
					require.Equal(t, http.MethodPost, r.Method)
					_, _ = w.Write([]byte(tt.init))
				default:
					_, _ = w.Write([]byte(`{"accounts":{"default":{"account":{"account_id":"org1","plan_type":"plus"}}}}`))
				}
			}))
			defer server.Close()
			client := &chatGPTAccountClient{client: server.Client(), baseURL: server.URL}
			snapshot, err := client.sync(context.Background(), "at-secret")
			require.NoError(t, err)
			require.Equal(t, "plus", snapshot.Plan)
			require.Equal(t, "test@example.com", snapshot.Email)
			require.Equal(t, tt.remaining, snapshot.QuotaRemaining)
			if tt.remaining != nil {
				_, err := time.Parse(time.RFC3339, snapshot.QuotaResetAt)
				require.NoError(t, err)
			}
		})
	}
}

func TestChatGPTRefreshKeepsMissingRotatedRTAndSanitizesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Equal(t, "old-rt-secret", r.Form.Get("refresh_token"))
		require.Equal(t, "custom-client", r.Form.Get("client_id"))
		if r.URL.Path == "/fail" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"old-rt-secret at-secret"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"new-at","expires_in":3600}`))
	}))
	defer server.Close()
	a := &Account{Credentials: map[string]any{"refresh_token": "old-rt-secret", "client_id": "custom-client"}}
	client := &chatGPTAccountClient{client: server.Client(), tokenURL: server.URL}
	credentials, err := client.refresh(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, "new-at", credentials["access_token"])
	require.NotContains(t, credentials, "refresh_token", "absence must preserve the stored RT during JSON merge")
	client.tokenURL = server.URL + "/fail"
	_, err = client.refresh(context.Background(), a)
	var upstream *chatGPTUpstreamError
	require.True(t, errors.As(err, &upstream))
	require.True(t, upstream.terminal)
	require.NotContains(t, err.Error(), "secret")
}

func TestChatGPTSyncDoesNotConvertFailuresIntoEmptyQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(strings.Repeat("sensitive-token", 10)))
	}))
	defer server.Close()
	client := &chatGPTAccountClient{client: server.Client(), baseURL: server.URL}
	snapshot, err := client.sync(context.Background(), "at-secret")
	require.Nil(t, snapshot)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "sensitive-token")
}
