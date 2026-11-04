package admin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatGPTImportArchivesAndExpiredRecoverableToken(t *testing.T) {
	expired := "header." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"email":"demo@example.com"}`, time.Now().Add(-time.Hour).Unix()))) + ".signature"
	for _, content := range []string{
		fmt.Sprintf(`{"accounts":[{"access_token":%q,"refresh_token":"rt"}]}`, expired),
		fmt.Sprintf(`{"data":{"items":[{"credentials":{"access_token":%q,"refresh_token":"rt"}}]}}`, expired),
	} {
		values, err := parseCodexSessionImportContent(content)
		require.NoError(t, err)
		values = chatGPTImportValues(values)
		require.Len(t, values, 1)
		entry := codexImportEntry{Index: 1, Value: values[0]}
		item, err := normalizeCodexImportEntryAllowExpired(entry, true)
		require.NoError(t, err)
		require.Equal(t, expired, item.AccessToken)
		require.Equal(t, "rt", item.RefreshToken)
		require.NotNil(t, item.TokenExpiresAt)
		require.True(t, item.TokenExpiresAt.Before(time.Now()))
		_, err = normalizeCodexImportEntry(entry)
		require.Error(t, err, "Codex import must continue rejecting expired tokens")
	}
}

func TestChatGPTBackupRestoresGroupsAndDisabledState(t *testing.T) {
	svc := &chatGPTBackupAdminStub{newStubAdminService()}
	svc.groups = []service.Group{{ID: 7, Name: "images", Platform: service.PlatformChatGPT}}
	h := &AccountHandler{adminService: svc}
	result, err := h.importData(context.Background(), DataImportRequest{Data: DataPayload{
		Accounts: []DataAccount{{Name: "backup", Platform: service.PlatformChatGPT, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "at", "refresh_token": "rt"}, Concurrency: 1,
			ChatGPTConfig: &DataChatGPTConfig{GroupNames: []string{"images"}, Status: "inactive", Schedulable: false}}},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.AccountCreated)
	require.Equal(t, []int64{7}, svc.createdAccounts[0].GroupIDs)
	require.NotNil(t, svc.lastUpdateAccountInput)
	require.Equal(t, "inactive", svc.lastUpdateAccountInput.Status)
}

type chatGPTBackupAdminStub struct{ *stubAdminService }

func (s *chatGPTBackupAdminStub) CreateAccount(ctx context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	account, err := s.stubAdminService.CreateAccount(ctx, input)
	if account != nil {
		account.Platform = input.Platform
	}
	return account, err
}

func TestChatGPTImportPreservesWebClientAndPlan(t *testing.T) {
	svc := newStubAdminService()
	h := &AccountHandler{adminService: svc}
	values := chatGPTImportValues([]any{map[string]any{"access_token": "at", "refresh_token": "rt", "type": "Plus", "notes": "image pool"}})
	result, err := h.importChatGPTAccounts(context.Background(), chatGPTImportRequest{Source: "web"}, values)
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, service.PlatformChatGPT, svc.createdAccounts[0].Platform)
	require.Equal(t, "app_2SKx67EdpoN0G6j64rFvigXD", svc.createdAccounts[0].Credentials["client_id"])
	require.Equal(t, "Plus", svc.createdAccounts[0].Credentials["plan_type"])
	require.Equal(t, "image pool", *svc.createdAccounts[0].Notes)
}

func TestChatGPTRemoteImportRejectsUnsafeBaseURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, base := range []string{"http://example.com", "file:///tmp/accounts", "https://user:pass@example.com", "https://example.com?key=secret", "https://example.com/#fragment"} {
		router := gin.New()
		h := &AccountHandler{}
		router.POST("/import", h.ImportRemoteChatGPTAccounts)
		body, err := json.Marshal(map[string]string{"kind": "cpa", "base_url": base, "api_key": "key-secret"})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/import", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.NotContains(t, w.Body.String(), "key-secret")
	}
}
