package repository

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// A database lease serializes RT rotations across server instances and expires
// after a crash. Its owner key prevents one caller releasing another's lease.
func (r *accountRepository) AcquireChatGPTMaintenance(ctx context.Context, id int64, owner string) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.AcquireChatGPTMaintenance(ctx, id, owner)
	}
	result, err := r.sql.ExecContext(ctx, `UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) ||
		jsonb_build_object('chatgpt_maintenance_owner', $2::text, 'chatgpt_maintenance_until', EXTRACT(EPOCH FROM NOW()) + 120)
		WHERE id = $1 AND platform = 'chatgpt' AND deleted_at IS NULL
		AND (CASE WHEN jsonb_typeof(extra->'chatgpt_maintenance_until') = 'number'
		THEN (extra->>'chatgpt_maintenance_until')::numeric ELSE 0 END) < EXTRACT(EPOCH FROM NOW())`, id, owner)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *accountRepository) ReleaseChatGPTMaintenance(ctx context.Context, id int64, owner string) error {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.ReleaseChatGPTMaintenance(ctx, id, owner)
	}
	_, err := r.sql.ExecContext(ctx, `UPDATE accounts SET extra = extra - 'chatgpt_maintenance_owner' - 'chatgpt_maintenance_until'
		WHERE id = $1 AND platform = 'chatgpt' AND extra->>'chatgpt_maintenance_owner' = $2`, id, owner)
	return err
}

// Merge only maintenance-owned keys, provided credentials and routing still
// match the remote request. Concurrent edits and soft deletion win over it.
func (r *accountRepository) UpdateChatGPTAccountState(ctx context.Context, expected *service.Account, credentials, extra map[string]any) (bool, error) {
	if scoped := r.accountTransactionScope(ctx); scoped != r {
		return scoped.UpdateChatGPTAccountState(ctx, expected, credentials, extra)
	}
	credsJSON, err := json.Marshal(credentials)
	if err != nil {
		return false, err
	}
	extraJSON, err := json.Marshal(extra)
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `
		UPDATE accounts SET credentials = COALESCE(credentials, '{}'::jsonb) || $1::jsonb,
			extra = COALESCE(extra, '{}'::jsonb) || $2::jsonb, updated_at = NOW()
		WHERE id = $3 AND platform = 'chatgpt' AND deleted_at IS NULL
			AND COALESCE(credentials->>'access_token', '') = $4
			AND COALESCE(credentials->>'refresh_token', '') = $5
			AND proxy_id IS NOT DISTINCT FROM $6
			AND updated_at = $7
			AND extra->>'chatgpt_maintenance_owner' = $8`, string(credsJSON), string(extraJSON), expected.ID,
		expected.GetCredential("access_token"), expected.GetCredential("refresh_token"), expected.ProxyID, expected.UpdatedAt, expected.Extra["chatgpt_maintenance_owner"])
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count > 0 {
		r.syncSchedulerAccountSnapshot(ctx, expected.ID)
	}
	return count > 0, nil
}
