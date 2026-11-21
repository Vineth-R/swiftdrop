package repository

import (
	"context"
	"fmt"
)

// PendingIdentitySyncs returns, by merchant ID, the merchants whose current
// REJECTED or SUSPENDED status has not yet reached Identity: the status was
// committed here, but the Identity call that deactivates the account and revokes
// its sessions failed and still needs a retry.
//
// Only the latest transition that produced the merchant's current status is
// looked at, so an earlier failed transition that a later one superseded never
// counts. A merchant that is PENDING or APPROVED is never listed: an approval
// commits only after Identity applied it.
func (r *Repository) PendingIdentitySyncs(ctx context.Context, merchantIDs []string) (map[string]bool, error) {
	pending := make(map[string]bool, len(merchantIDs))
	if len(merchantIDs) == 0 {
		return pending, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (t.merchant_id) t.merchant_id::text, t.sync_status
		FROM merchant.merchant_status_transitions t
		JOIN merchant.merchants m ON m.id = t.merchant_id AND t.to_status = m.status
		WHERE t.merchant_id = ANY($1::uuid[])
		  AND m.status IN ('REJECTED', 'SUSPENDED')
		ORDER BY t.merchant_id, t.sequence DESC`, merchantIDs)
	if err != nil {
		return nil, fmt.Errorf("reading pending identity syncs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var merchantID, syncStatus string
		if err := rows.Scan(&merchantID, &syncStatus); err != nil {
			return nil, fmt.Errorf("reading pending identity syncs: %w", err)
		}
		if syncStatus != "APPLIED" {
			pending[merchantID] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading pending identity syncs: %w", err)
	}
	return pending, nil
}
