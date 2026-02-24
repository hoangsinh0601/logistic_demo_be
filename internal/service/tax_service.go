package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"backend/internal/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// --- DTOs ---

type CreateTaxRuleRequest struct {
	TaxType       string `json:"tax_type" binding:"required,oneof=VAT_INLAND VAT_INTL FCT"`
	Rate          string `json:"rate" binding:"required"`           // Decimal string, e.g. "0.10"
	EffectiveFrom string `json:"effective_from" binding:"required"` // YYYY-MM-DD
	EffectiveTo   string `json:"effective_to"`                      // YYYY-MM-DD, nullable
	Description   string `json:"description"`
}

type TaxRuleResponse struct {
	ID            string  `json:"id"`
	TaxType       string  `json:"tax_type"`
	Rate          string  `json:"rate"`
	EffectiveFrom string  `json:"effective_from"`
	EffectiveTo   *string `json:"effective_to"`
	Description   string  `json:"description"`
	CreatedAt     string  `json:"created_at"`
}

// --- Interface ---

type TaxService interface {
	GetTaxRules(ctx context.Context) ([]TaxRuleResponse, error)
	CreateTaxRule(ctx context.Context, req CreateTaxRuleRequest) (TaxRuleResponse, error)
	CalculateActiveTax(ctx context.Context, taxType string, targetDate time.Time) (decimal.Decimal, error)
}

type taxService struct {
	db *gorm.DB
}

func NewTaxService(db *gorm.DB) TaxService {
	return &taxService{db: db}
}

// --- Implementation ---

func (s *taxService) GetTaxRules(ctx context.Context) ([]TaxRuleResponse, error) {
	var rules []model.TaxRule
	if err := s.db.WithContext(ctx).Order("effective_from DESC").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch tax rules: %w", err)
	}

	res := make([]TaxRuleResponse, 0, len(rules))
	for _, r := range rules {
		resp := TaxRuleResponse{
			ID:            r.ID.String(),
			TaxType:       r.TaxType,
			Rate:          r.Rate.StringFixed(4),
			EffectiveFrom: r.EffectiveFrom.Format("2006-01-02"),
			Description:   r.Description,
			CreatedAt:     r.CreatedAt.Format(time.RFC3339),
		}
		if r.EffectiveTo != nil {
			s := r.EffectiveTo.Format("2006-01-02")
			resp.EffectiveTo = &s
		}
		res = append(res, resp)
	}

	return res, nil
}

func (s *taxService) CreateTaxRule(ctx context.Context, req CreateTaxRuleRequest) (TaxRuleResponse, error) {
	rate, err := decimal.NewFromString(req.Rate)
	if err != nil {
		return TaxRuleResponse{}, fmt.Errorf("invalid rate value: %w", err)
	}

	effectiveFrom, err := time.Parse("2006-01-02", req.EffectiveFrom)
	if err != nil {
		return TaxRuleResponse{}, fmt.Errorf("invalid effective_from date format (expected YYYY-MM-DD): %w", err)
	}

	var effectiveTo *time.Time
	if req.EffectiveTo != "" {
		t, err := time.Parse("2006-01-02", req.EffectiveTo)
		if err != nil {
			return TaxRuleResponse{}, fmt.Errorf("invalid effective_to date format (expected YYYY-MM-DD): %w", err)
		}
		effectiveTo = &t
	}

	rule := model.TaxRule{
		TaxType:       req.TaxType,
		Rate:          rate,
		EffectiveFrom: effectiveFrom,
		EffectiveTo:   effectiveTo,
		Description:   req.Description,
	}

	if err := s.db.WithContext(ctx).Create(&rule).Error; err != nil {
		return TaxRuleResponse{}, fmt.Errorf("failed to create tax rule: %w", err)
	}

	resp := TaxRuleResponse{
		ID:            rule.ID.String(),
		TaxType:       rule.TaxType,
		Rate:          rule.Rate.StringFixed(4),
		EffectiveFrom: rule.EffectiveFrom.Format("2006-01-02"),
		Description:   rule.Description,
		CreatedAt:     rule.CreatedAt.Format(time.RFC3339),
	}
	if rule.EffectiveTo != nil {
		s := rule.EffectiveTo.Format("2006-01-02")
		resp.EffectiveTo = &s
	}

	return resp, nil
}

// CalculateActiveTax finds the active tax rate for a given type and date.
// Query: effective_from <= targetDate AND (effective_to IS NULL OR effective_to >= targetDate)
func (s *taxService) CalculateActiveTax(ctx context.Context, taxType string, targetDate time.Time) (decimal.Decimal, error) {
	var rule model.TaxRule

	err := s.db.WithContext(ctx).
		Where("tax_type = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)",
			taxType, targetDate, targetDate).
		Order("effective_from DESC").
		First(&rule).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return decimal.Zero, fmt.Errorf("no active tax rule found for type '%s' on date %s", taxType, targetDate.Format("2006-01-02"))
		}
		return decimal.Zero, fmt.Errorf("failed to query tax rule: %w", err)
	}

	return rule.Rate, nil
}

// Ensure uuid import is used in DTO context (compiler safeguard)
var _ = uuid.New
