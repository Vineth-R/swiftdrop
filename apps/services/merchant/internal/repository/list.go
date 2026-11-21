package repository

import (
	"context"
	"fmt"
	"strings"
)

// MerchantFilter selects the admin merchant list.
type MerchantFilter struct {
	Status string
	Search string
	Sort   string
	Desc   bool
	Limit  int
	Offset int
}

// ListMerchants returns a page of merchants and the total count.
func (r *Repository) ListMerchants(ctx context.Context, filter MerchantFilter) ([]Merchant, int, error) {
	where := []string{"1=1"}
	args := []any{}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
		args = append(args, "%"+escaped+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(business_name ILIKE $%d ESCAPE '\\' OR business_reg_no ILIKE $%d ESCAPE '\\' OR contact_email ILIKE $%d ESCAPE '\\')", n, n, n))
	}
	sortColumn := "created_at"
	switch filter.Sort {
	case "businessName":
		sortColumn = "lower(business_name)"
	case "status":
		sortColumn = "status"
	case "createdAt", "":
		sortColumn = "created_at"
	}
	direction := "ASC"
	if filter.Desc {
		direction = "DESC"
	}
	args = append(args, filter.Limit, filter.Offset)
	query := fmt.Sprintf(`
		SELECT `+merchantColumns+`
		FROM merchant.merchants
		WHERE %s
		ORDER BY %s %s, id
		LIMIT $%d OFFSET $%d`,
		strings.Join(where, " AND "), sortColumn, direction, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing merchants: %w", err)
	}
	defer rows.Close()
	merchants := make([]Merchant, 0)
	for rows.Next() {
		merchant, err := scanMerchant(rows)
		if err != nil {
			return nil, 0, err
		}
		merchants = append(merchants, merchant)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listing merchants: %w", err)
	}
	countQuery := fmt.Sprintf(`SELECT count(*) FROM merchant.merchants WHERE %s`, strings.Join(where, " AND "))
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args[:len(args)-2]...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting merchants: %w", err)
	}
	return merchants, total, nil
}
