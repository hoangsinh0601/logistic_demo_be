package model

import (
	"time"
)

// StatisticsResponse aggregates transaction totals and ranking data
type StatisticsResponse struct {
	TotalImportValue   float64          `json:"total_import_value"`
	TotalExportValue   float64          `json:"total_export_value"`
	TotalImportOrders  int              `json:"total_import_orders"`
	TotalExportOrders  int              `json:"total_export_orders"`
	Profit             float64          `json:"profit"`
	TopImportedItems   []ProductRanking `json:"top_imported_items"`
	TopExportedItems   []ProductRanking `json:"top_exported_items"`
	TimeRangeStartDate time.Time        `json:"time_range_start_date"`
	TimeRangeEndDate   time.Time        `json:"time_range_end_date"`
}

// ProductRanking represents a ranked product based on accumulated quantities
type ProductRanking struct {
	ProductID     string  `json:"product_id"`
	ProductName   string  `json:"product_name"`
	ProductSKU    string  `json:"product_sku"`
	TotalQuantity int     `json:"total_quantity"`
	TotalValue    float64 `json:"total_value"`
}
