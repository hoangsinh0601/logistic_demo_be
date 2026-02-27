package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// StatisticsResponse aggregates transaction totals and ranking data
type StatisticsResponse struct {
	TotalImportValue   decimal.Decimal  `json:"total_import_value"`
	TotalExportValue   decimal.Decimal  `json:"total_export_value"`
	TotalImportOrders  int              `json:"total_import_orders"`
	TotalExportOrders  int              `json:"total_export_orders"`
	Profit             decimal.Decimal  `json:"profit"`
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
