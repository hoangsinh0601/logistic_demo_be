package service

import (
	"context"
	"fmt"

	"backend/internal/model"

	"gorm.io/gorm"
)

// --- DTOs ---

type RevenueDataPoint struct {
	Period            string `json:"period"`
	TotalRevenue      string `json:"total_revenue"`
	TotalExpense      string `json:"total_expense"`
	TotalTaxCollected string `json:"total_tax_collected"`
	TotalTaxPaid      string `json:"total_tax_paid"`
	TotalSideFees     string `json:"total_side_fees"`
}

type RevenueFilter struct {
	GroupBy   string // week, month, quarter
	StartDate string // RFC3339
	EndDate   string // RFC3339
}

// --- Interface ---

type RevenueService interface {
	GetRevenueStatistics(ctx context.Context, filter RevenueFilter) ([]RevenueDataPoint, error)
}

type revenueService struct {
	db *gorm.DB
}

func NewRevenueService(db *gorm.DB) RevenueService {
	return &revenueService{db: db}
}

// --- Implementation ---

func (s *revenueService) GetRevenueStatistics(ctx context.Context, filter RevenueFilter) ([]RevenueDataPoint, error) {
	// Validate group_by
	groupBy := filter.GroupBy
	switch groupBy {
	case "week", "month", "quarter", "year":
		// valid
	default:
		groupBy = "month" // default
	}

	// Build raw SQL using DATE_TRUNC — only APPROVED invoices
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

	type rawResult struct {
		Period            string  `gorm:"column:period"`
		TotalRevenue      float64 `gorm:"column:total_revenue"`
		TotalExpense      float64 `gorm:"column:total_expense"`
		TotalTaxCollected float64 `gorm:"column:total_tax_collected"`
		TotalTaxPaid      float64 `gorm:"column:total_tax_paid"`
		TotalSideFees     float64 `gorm:"column:total_side_fees"`
	}

	var rows []rawResult
	if err := s.db.WithContext(ctx).Raw(query,
		groupBy,
		filter.StartDate,
		filter.EndDate,
		model.RefTypeOrderExport,
		model.RefTypeOrderImport,
		model.RefTypeExpense,
		model.ApprovalApproved,
	).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query revenue statistics: %w", err)
	}

	result := make([]RevenueDataPoint, 0, len(rows))
	for _, r := range rows {
		result = append(result, RevenueDataPoint{
			Period:            r.Period,
			TotalRevenue:      fmt.Sprintf("%.4f", r.TotalRevenue),
			TotalExpense:      fmt.Sprintf("%.4f", r.TotalExpense),
			TotalTaxCollected: fmt.Sprintf("%.4f", r.TotalTaxCollected),
			TotalTaxPaid:      fmt.Sprintf("%.4f", r.TotalTaxPaid),
			TotalSideFees:     fmt.Sprintf("%.4f", r.TotalSideFees),
		})
	}

	return result, nil
}
