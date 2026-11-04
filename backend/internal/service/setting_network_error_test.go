//go:build unit

package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNetworkErrorCooldownSettings(t *testing.T) {
	ctx := context.Background()
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})
	for _, raw := range []string{"", "{}", "invalid"} {
		repo.data[SettingKeyNetworkErrorCooldownSettings] = raw
		settings, err := svc.GetNetworkErrorCooldownSettings(ctx)
		require.NoError(t, err)
		require.True(t, settings.Enabled)
	}
	for _, enabled := range []bool{false, true} {
		require.NoError(t, svc.SetNetworkErrorCooldownSettings(ctx, &NetworkErrorCooldownSettings{Enabled: enabled}))
		settings, err := svc.GetNetworkErrorCooldownSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, enabled, settings.Enabled)
	}
}

func TestNetworkErrorCooldownPreservesFailover(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			ctx := context.Background()
			settingSvc := NewSettingService(newMockSettingRepo(), &config.Config{})
			require.NoError(t, settingSvc.SetNetworkErrorCooldownSettings(ctx, &NetworkErrorCooldownSettings{Enabled: enabled}))
			repo := &capacityShedAccountRepoStub{}
			svc := &OpenAIGatewayService{accountRepo: repo, settingService: settingSvc}
			account := &Account{ID: 123, Platform: "openai"}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			err := svc.handleOpenAIUpstreamTransportError(ctx, c, account, errors.New("connection refused"), false)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, 502, failover.StatusCode)
			_, blocked := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
			require.Equal(t, enabled, blocked)
			if enabled {
				require.Equal(t, 1, repo.tempUnschedCalls)
			} else {
				require.Zero(t, repo.tempUnschedCalls)
			}
		})
	}
}

func TestNetworkErrorCooldownDefaultsWhenSettingsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings *SettingService
	}{
		{name: "no service"},
		{name: "no repository", settings: &SettingService{}},
		{name: "repository error", settings: NewSettingService(&mockSettingRepo{getValueErr: errors.New("database unavailable")}, &config.Config{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &capacityShedAccountRepoStub{}
			svc := &OpenAIGatewayService{accountRepo: repo, settingService: tc.settings}
			account := &Account{ID: 123, Platform: "openai"}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account, errors.New("connection refused"), false)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			_, blocked := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
			require.True(t, blocked)
			require.Equal(t, 1, repo.tempUnschedCalls)
		})
	}
}
