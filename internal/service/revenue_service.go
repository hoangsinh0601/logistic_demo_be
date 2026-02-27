package service

import (
	"context"
	"fmt"

	"backend/internal/model"
	"backend/internal/repository"
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
	revenueRepo repository.RevenueRepository
}

func NewRevenueService(revenueRepo repository.RevenueRepository) RevenueService {
	return &revenueService{revenueRepo: revenueRepo}
}

// --- Implementation ---

func (s *revenueService) GetRevenueStatistics(ctx context.Context, filter RevenueFilter) ([]RevenueDataPoint, error) {
	groupBy := filter.GroupBy
	switch groupBy {
	case "week", "month", "quarter", "year":
		// valid
	default:
		groupBy = "month"
	}

	rows, err := s.revenueRepo.GetRevenueStatistics(ctx,
		groupBy, filter.StartDate, filter.EndDate,
		model.RefTypeOrderExport, model.RefTypeOrderImport, model.RefTypeExpense, model.ApprovalApproved,
	)
	if err != nil {
		return nil, err
	}

	result := make([]RevenueDataPoint, 0, len(rows))
	for _, r := range rows {
		result = append(result, RevenueDataPoint{
			Period:            fmt.Sprintf("%.0f", r.Period),
			TotalRevenue:      fmt.Sprintf("%.4f", r.TotalRevenue),
			TotalExpense:      fmt.Sprintf("%.4f", r.TotalExpense),
			TotalTaxCollected: fmt.Sprintf("%.4f", r.TotalTaxCollected),
			TotalTaxPaid:      fmt.Sprintf("%.4f", r.TotalTaxPaid),
			TotalSideFees:     fmt.Sprintf("%.4f", r.TotalSideFees),
		})
	}

	return result, nil
}
