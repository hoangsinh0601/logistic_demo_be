package service

import (
	"context"
	"time"

	"backend/internal/model"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type StatisticsService interface {
	GetStatistics(ctx context.Context, startDate, endDate time.Time) (model.StatisticsResponse, error)
}

type statisticsService struct {
	db *gorm.DB
}

func NewStatisticsService(db *gorm.DB) StatisticsService {
	return &statisticsService{db: db}
}

// GetStatistics aggregated metrics bounding valid Order structures into time brackets
func (s *statisticsService) GetStatistics(ctx context.Context, startDate, endDate time.Time) (model.StatisticsResponse, error) {
	var response model.StatisticsResponse
	response.TimeRangeStartDate = startDate
	response.TimeRangeEndDate = endDate

	// Calculate Total Import Value & Count
	var totalImport struct {
		Value string
		Count int
	}
	s.db.WithContext(ctx).Table("order_items").
		Select("COALESCE(CAST(SUM(order_items.quantity * order_items.unit_price) AS TEXT), '0') as value, COUNT(DISTINCT orders.id) as count").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.type = ? AND orders.status = ? AND orders.created_at >= ? AND orders.created_at <= ?", model.OrderTypeImport, "COMPLETED", startDate, endDate).
		Scan(&totalImport)

	importVal, _ := decimal.NewFromString(totalImport.Value)
	response.TotalImportValue = importVal
	response.TotalImportOrders = totalImport.Count

	// Calculate Total Export Value & Count
	var totalExport struct {
		Value string
		Count int
	}
	s.db.WithContext(ctx).Table("order_items").
		Select("COALESCE(CAST(SUM(order_items.quantity * order_items.unit_price) AS TEXT), '0') as value, COUNT(DISTINCT orders.id) as count").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.type = ? AND orders.status = ? AND orders.created_at >= ? AND orders.created_at <= ?", model.OrderTypeExport, "COMPLETED", startDate, endDate).
		Scan(&totalExport)

	exportVal, _ := decimal.NewFromString(totalExport.Value)
	response.TotalExportValue = exportVal
	response.TotalExportOrders = totalExport.Count

	// Profit = Export Value - Import Value
	response.Profit = exportVal.Sub(importVal)

	// Calculate Top Imported Items
	var topImports []model.ProductRanking
	s.db.WithContext(ctx).Table("order_items").
		Select("products.id as product_id, products.name as product_name, products.sku as product_sku, SUM(order_items.quantity) as total_quantity, SUM(order_items.quantity * order_items.unit_price) as total_value").
		Joins("JOIN products ON products.id = order_items.product_id").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.type = ? AND orders.status = ? AND orders.created_at >= ? AND orders.created_at <= ?", model.OrderTypeImport, "COMPLETED", startDate, endDate).
		Group("products.id, products.name, products.sku").
		Order("total_quantity DESC").
		Limit(5).
		Scan(&topImports)
	response.TopImportedItems = topImports

	// Calculate Top Exported Items
	var topExports []model.ProductRanking
	s.db.WithContext(ctx).Table("order_items").
		Select("products.id as product_id, products.name as product_name, products.sku as product_sku, SUM(order_items.quantity) as total_quantity, SUM(order_items.quantity * order_items.unit_price) as total_value").
		Joins("JOIN products ON products.id = order_items.product_id").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.type = ? AND orders.status = ? AND orders.created_at >= ? AND orders.created_at <= ?", model.OrderTypeExport, "COMPLETED", startDate, endDate).
		Group("products.id, products.name, products.sku").
		Order("total_quantity DESC").
		Limit(5).
		Scan(&topExports)
	response.TopExportedItems = topExports

	return response, nil
}
