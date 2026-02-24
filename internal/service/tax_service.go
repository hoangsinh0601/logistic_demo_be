package service

import (
	"context"
	"encoding/json"
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

type UpdateTaxRuleRequest struct {
	TaxType       string `json:"tax_type" binding:"required,oneof=VAT_INLAND VAT_INTL FCT"`
	Rate          string `json:"rate" binding:"required"`
	EffectiveFrom string `json:"effective_from" binding:"required"`
	EffectiveTo   string `json:"effective_to"`
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

type ActiveTaxRateResponse struct {
	TaxType string `json:"tax_type"`
	Rate    string `json:"rate"`
	RuleID  string `json:"rule_id"`
}

// --- Interface ---

type TaxService interface {
	GetTaxRules(ctx context.Context) ([]TaxRuleResponse, error)
	CreateTaxRule(ctx context.Context, req CreateTaxRuleRequest, userID string) (TaxRuleResponse, error)
	UpdateTaxRule(ctx context.Context, id string, req UpdateTaxRuleRequest, userID string) (TaxRuleResponse, error)
	DeleteTaxRule(ctx context.Context, id string, userID string) error
	GetActiveTaxRate(ctx context.Context, taxType string) (*ActiveTaxRateResponse, error)
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
		res = append(res, toTaxRuleResponse(r))
	}

	return res, nil
}

func (s *taxService) CreateTaxRule(ctx context.Context, req CreateTaxRuleRequest, userID string) (TaxRuleResponse, error) {
	rate, effectiveFrom, effectiveTo, err := parseTaxRuleFields(req.Rate, req.EffectiveFrom, req.EffectiveTo)
	if err != nil {
		return TaxRuleResponse{}, err
	}

	// Validate overlap
	if err := s.checkOverlap(ctx, req.TaxType, effectiveFrom, effectiveTo, nil); err != nil {
		return TaxRuleResponse{}, err
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

	// Audit log
	s.writeAuditLog(ctx, userID, model.ActionCreateTaxRule, rule.ID.String(), req.TaxType+" "+rate.StringFixed(4), req)

	return toTaxRuleResponse(rule), nil
}

func (s *taxService) UpdateTaxRule(ctx context.Context, id string, req UpdateTaxRuleRequest, userID string) (TaxRuleResponse, error) {
	ruleID, err := uuid.Parse(id)
	if err != nil {
		return TaxRuleResponse{}, fmt.Errorf("invalid tax rule id: %w", err)
	}

	var rule model.TaxRule
	if err := s.db.WithContext(ctx).First(&rule, "id = ?", ruleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return TaxRuleResponse{}, fmt.Errorf("tax rule not found")
		}
		return TaxRuleResponse{}, fmt.Errorf("failed to fetch tax rule: %w", err)
	}

	rate, effectiveFrom, effectiveTo, err := parseTaxRuleFields(req.Rate, req.EffectiveFrom, req.EffectiveTo)
	if err != nil {
		return TaxRuleResponse{}, err
	}

	// Validate overlap (exclude self)
	if err := s.checkOverlap(ctx, req.TaxType, effectiveFrom, effectiveTo, &ruleID); err != nil {
		return TaxRuleResponse{}, err
	}

	rule.TaxType = req.TaxType
	rule.Rate = rate
	rule.EffectiveFrom = effectiveFrom
	rule.EffectiveTo = effectiveTo
	rule.Description = req.Description

	if err := s.db.WithContext(ctx).Save(&rule).Error; err != nil {
		return TaxRuleResponse{}, fmt.Errorf("failed to update tax rule: %w", err)
	}

	// Audit log
	s.writeAuditLog(ctx, userID, model.ActionUpdateTaxRule, rule.ID.String(), req.TaxType+" "+rate.StringFixed(4), req)

	return toTaxRuleResponse(rule), nil
}

func (s *taxService) DeleteTaxRule(ctx context.Context, id string, userID string) error {
	ruleID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid tax rule id: %w", err)
	}

	var rule model.TaxRule
	if err := s.db.WithContext(ctx).First(&rule, "id = ?", ruleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("tax rule not found")
		}
		return fmt.Errorf("failed to fetch tax rule: %w", err)
	}

	if err := s.db.WithContext(ctx).Delete(&rule).Error; err != nil {
		return fmt.Errorf("failed to delete tax rule: %w", err)
	}

	// Audit log
	s.writeAuditLog(ctx, userID, model.ActionDeleteTaxRule, rule.ID.String(), rule.TaxType+" "+rule.Rate.StringFixed(4), map[string]string{"deleted_id": id})

	return nil
}

func (s *taxService) GetActiveTaxRate(ctx context.Context, taxType string) (*ActiveTaxRateResponse, error) {
	var rule model.TaxRule
	now := time.Now()

	err := s.db.WithContext(ctx).
		Where("tax_type = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)",
			taxType, now, now).
		Order("effective_from DESC").
		First(&rule).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No active rate — not an error
		}
		return nil, fmt.Errorf("failed to query active tax rate: %w", err)
	}

	return &ActiveTaxRateResponse{
		TaxType: rule.TaxType,
		Rate:    rule.Rate.StringFixed(4),
		RuleID:  rule.ID.String(),
	}, nil
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

// --- Helpers ---

func parseTaxRuleFields(rateStr, fromStr, toStr string) (decimal.Decimal, time.Time, *time.Time, error) {
	rate, err := decimal.NewFromString(rateStr)
	if err != nil {
		return decimal.Zero, time.Time{}, nil, fmt.Errorf("invalid rate value: %w", err)
	}

	effectiveFrom, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return decimal.Zero, time.Time{}, nil, fmt.Errorf("invalid effective_from date format (expected YYYY-MM-DD): %w", err)
	}

	var effectiveTo *time.Time
	if toStr != "" {
		t, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			return decimal.Zero, time.Time{}, nil, fmt.Errorf("invalid effective_to date format (expected YYYY-MM-DD): %w", err)
		}
		effectiveTo = &t
	}

	return rate, effectiveFrom, effectiveTo, nil
}

func (s *taxService) checkOverlap(ctx context.Context, taxType string, from time.Time, to *time.Time, excludeID *uuid.UUID) error {
	query := s.db.WithContext(ctx).Model(&model.TaxRule{}).
		Where("tax_type = ?", taxType).
		Where("effective_from <= ?", func() time.Time {
			if to != nil {
				return *to
			}
			return time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		}()).
		Where("(effective_to IS NULL OR effective_to >= ?)", from)

	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check overlap: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("a tax rule for '%s' already exists with overlapping effective dates", taxType)
	}

	return nil
}

func toTaxRuleResponse(r model.TaxRule) TaxRuleResponse {
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
	return resp
}

func (s *taxService) writeAuditLog(ctx context.Context, userID, action, entityID, entityName string, details interface{}) {
	detailsJSON, _ := json.Marshal(details)

	log := model.AuditLog{
		Action:     action,
		EntityID:   entityID,
		EntityName: entityName,
		Details:    string(detailsJSON),
	}

	if userID != "" {
		parsed, err := uuid.Parse(userID)
		if err == nil {
			log.UserID = &parsed
		}
	}

	// Best-effort audit log — don't fail the operation if logging fails
	_ = s.db.WithContext(ctx).Create(&log).Error
}

// Ensure uuid import is used in DTO context (compiler safeguard)
var _ = uuid.New
