package repository

import (
	"context"
	"fmt"
	"time"

	"backend/internal/model"

	"gorm.io/gorm"
)

type StatisticsRepository interface {
	GetOrderStatistics(ctx context.Context, orderType, status string, start, end time.Time) (value string, count int, err error)
	GetTopProducts(ctx context.Context, orderType, status string, start, end time.Time, limit int) ([]model.ProductRanking, error)
}

type statisticsRepository struct {
	db *gorm.DB
}

func NewStatisticsRepository(db *gorm.DB) StatisticsRepository {
	return &statisticsRepository{db: db}
}

func (r *statisticsRepository) GetOrderStatistics(ctx context.Context, orderType, status string, start, end time.Time) (string, int, error) {
	var result struct {
		Value string
		Count int
	}
	r.db.WithContext(ctx).Table("order_items").
		Select("COALESCE(CAST(SUM(order_items.quantity * order_items.unit_price) AS TEXT), '0') as value, COUNT(DISTINCT orders.id) as count").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.type = ? AND orders.status = ? AND orders.created_at >= ? AND orders.created_at <= ?", orderType, status, start, end).
		Scan(&result)
	return result.Value, result.Count, nil
}

func (r *statisticsRepository) GetTopProducts(ctx context.Context, orderType, status string, start, end time.Time, limit int) ([]model.ProductRanking, error) {
	var rankings []model.ProductRanking
	if err := r.db.WithContext(ctx).Table("order_items").
		Select("products.id as product_id, products.name as product_name, products.sku as product_sku, SUM(order_items.quantity) as total_quantity, SUM(order_items.quantity * order_items.unit_price) as total_value").
		Joins("JOIN products ON products.id = order_items.product_id").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.type = ? AND orders.status = ? AND orders.created_at >= ? AND orders.created_at <= ?", orderType, status, start, end).
		Group("products.id, products.name, products.sku").
		Order("total_quantity DESC").
		Limit(limit).
		Scan(&rankings).Error; err != nil {
		return nil, fmt.Errorf("failed to query top products: %w", err)
	}
	return rankings, nil
}
