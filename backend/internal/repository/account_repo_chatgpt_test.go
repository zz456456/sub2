package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChatGPTMaintenanceCASRejectsConcurrentEdits(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		t.Run(string(rune('0'+affected)), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			r := &accountRepository{sql: db}
			now := time.Now()
			proxyID := int64(8)
			expected := &service.Account{ID: 42, Platform: service.PlatformChatGPT, Credentials: map[string]any{"access_token": "at", "refresh_token": "rt"}, Extra: map[string]any{"chatgpt_maintenance_owner": "lease"}, ProxyID: &proxyID, UpdatedAt: now}
			mock.ExpectExec(`(?s)UPDATE accounts.*deleted_at IS NULL.*credentials->>'access_token'.*credentials->>'refresh_token'.*proxy_id IS NOT DISTINCT FROM.*updated_at =.*chatgpt_maintenance_owner`).
				WithArgs(`{"access_token":"new-at"}`, `{"chatgpt_access_invalid":false}`, int64(42), "at", "rt", proxyID, now, "lease").WillReturnResult(sqlmock.NewResult(0, affected))
			applied, err := r.UpdateChatGPTAccountState(context.Background(), expected, map[string]any{"access_token": "new-at"}, map[string]any{"chatgpt_access_invalid": false})
			require.NoError(t, err)
			require.Equal(t, affected == 1, applied)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestChatGPTMaintenanceLeaseHasBoundedExpiryAndOwnerRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	r := &accountRepository{sql: db}
	mock.ExpectExec(`(?s)UPDATE accounts.*chatgpt_maintenance_until.*120.*deleted_at IS NULL.*EXTRACT`).WithArgs(int64(9), "owner").WillReturnResult(sqlmock.NewResult(0, 1))
	acquired, err := r.AcquireChatGPTMaintenance(context.Background(), 9, "owner")
	require.NoError(t, err)
	require.True(t, acquired)
	mock.ExpectExec(`(?s)UPDATE accounts.*chatgpt_maintenance_owner.*WHERE.*chatgpt_maintenance_owner.*= \$2`).WithArgs(int64(9), "owner").WillReturnResult(sqlmock.NewResult(0, 0))
	require.NoError(t, r.ReleaseChatGPTMaintenance(context.Background(), 9, "owner"))
	require.NoError(t, mock.ExpectationsWereMet())
}
