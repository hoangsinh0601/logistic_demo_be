package service

import (
	"context"
	"time"

	"backend/internal/model"
	"backend/internal/repository"

	"github.com/shopspring/decimal"
)

type StatisticsService interface {
	GetStatistics(ctx context.Context, startDate, endDate time.Time) (model.StatisticsResponse, error)
}

type statisticsService struct {
	statsRepo repository.StatisticsRepository
}

func NewStatisticsService(statsRepo repository.StatisticsRepository) StatisticsService {
	return &statisticsService{statsRepo: statsRepo}
}

func (s *statisticsService) GetStatistics(ctx context.Context, startDate, endDate time.Time) (model.StatisticsResponse, error) {
	var response model.StatisticsResponse
	response.TimeRangeStartDate = startDate
	response.TimeRangeEndDate = endDate

	// Total Import
	importValue, importCount, _ := s.statsRepo.GetOrderStatistics(ctx, model.OrderTypeImport, "COMPLETED", startDate, endDate)
	importVal, _ := decimal.NewFromString(importValue)
	response.TotalImportValue = importVal
	response.TotalImportOrders = importCount

	// Total Export
	exportValue, exportCount, _ := s.statsRepo.GetOrderStatistics(ctx, model.OrderTypeExport, "COMPLETED", startDate, endDate)
	exportVal, _ := decimal.NewFromString(exportValue)
	response.TotalExportValue = exportVal
	response.TotalExportOrders = exportCount

	// Profit
	response.Profit = exportVal.Sub(importVal)

	// Top Products
	topImports, _ := s.statsRepo.GetTopProducts(ctx, model.OrderTypeImport, "COMPLETED", startDate, endDate, 5)
	response.TopImportedItems = topImports

	topExports, _ := s.statsRepo.GetTopProducts(ctx, model.OrderTypeExport, "COMPLETED", startDate, endDate, 5)
	response.TopExportedItems = topExports

	return response, nil
}
