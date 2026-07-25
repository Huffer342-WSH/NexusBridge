package storage

import "context"

// RecoverInterruptedSubscriptionWork 恢复上次进程退出时未完成的订阅领取。
func (s *SQLiteStore) RecoverInterruptedSubscriptionWork(ctx context.Context) error {
	tx, release, err := s.beginWriteTx(ctx)
	if err != nil {
		return err
	}
	defer release()
	defer rollbackUnlessCommitted(tx)
	if _, err := tx.ExecContext(ctx, `
UPDATE download_tasks
SET status = CASE
		WHEN retry_count > 0 AND EXISTS (
			SELECT 1 FROM subscription_candidates c
			WHERE c.site_id = download_tasks.site_id
				AND c.torrent_id = download_tasks.torrent_id
				AND c.status = 'failed'
		) THEN 'failed'
		ELSE 'pending'
	END,
	reason_code = 'execution_recovered',
	error = 'recovered after interrupted execution',
	updated_at = CURRENT_TIMESTAMP
WHERE status = 'processing'
`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE subscription_candidates
SET status = 'unread', reason_code = 'execution_recovered', error = 'recovered after interrupted execution', updated_at = CURRENT_TIMESTAMP
WHERE status = 'processing'
`); err != nil {
		return err
	}
	return tx.Commit()
}
