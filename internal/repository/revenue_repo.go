package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type RevenueDataRow struct {
	Period            string  `gorm:"column:period"`
	TotalRevenue      float64 `gorm:"column:total_revenue"`
	TotalExpense      float64 `gorm:"column:total_expense"`
	TotalTaxCollected float64 `gorm:"column:total_tax_collected"`
	TotalTaxPaid      float64 `gorm:"column:total_tax_paid"`
	TotalSideFees     float64 `gorm:"column:total_side_fees"`
}

type RevenueRepository interface {
	GetRevenueStatistics(ctx context.Context, groupBy, startDate, endDate, exportType, importType, expenseType, approvedStatus string) ([]RevenueDataRow, error)
}

type revenueRepository struct {
	db *gorm.DB
}

func NewRevenueRepository(db *gorm.DB) RevenueRepository {
	return &revenueRepository{db: db}
}

func (r *revenueRepository) GetRevenueStatistics(ctx context.Context, groupBy, startDate, endDate, exportType, importType, expenseType, approvedStatus string) ([]RevenueDataRow, error) {
	query := `
		SELECT
			TO_CHAR(DATE_TRUNC($1, i.created_at), 'YYYY-MM-DD') AS period,
			COALESCE(SUM(CASE WHEN i.reference_type = $4 THEN i.total_amount ELSE 0 END), 0) AS total_revenue,
			COALESCE(SUM(CASE WHEN i.reference_type IN ($5, $6) THEN i.total_amount ELSE 0 END), 0) AS total_expense,
			COALESCE(SUM(CASE WHEN i.reference_type = $4 THEN i.tax_amount ELSE 0 END), 0) AS total_tax_collected,
			COALESCE(SUM(CASE WHEN i.reference_type IN ($5, $6) THEN i.tax_amount ELSE 0 END), 0) AS total_tax_paid,
			COALESCE(SUM(i.side_fees), 0) AS total_side_fees
		FROM invoices i
		WHERE i.approval_status = $7
		  AND i.created_at >= $2::timestamptz
		  AND i.created_at <= $3::timestamptz
		GROUP BY DATE_TRUNC($1, i.created_at)
		ORDER BY period
	`

	var rows []RevenueDataRow
	if err := r.db.WithContext(ctx).Raw(query,
		groupBy, startDate, endDate, exportType, importType, expenseType, approvedStatus,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query revenue statistics: %w", err)
	}

	return rows, nil
}
