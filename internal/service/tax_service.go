package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"backend/internal/model"
	"backend/internal/repository"

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
	GetTaxRules(ctx context.Context, search string, page, limit int) ([]TaxRuleResponse, int64, error)
	CreateTaxRule(ctx context.Context, req CreateTaxRuleRequest, userID string) (TaxRuleResponse, error)
	UpdateTaxRule(ctx context.Context, id string, req UpdateTaxRuleRequest, userID string) (TaxRuleResponse, error)
	DeleteTaxRule(ctx context.Context, id string, userID string) error
	GetActiveTaxRate(ctx context.Context, taxType string) (*ActiveTaxRateResponse, error)
	CalculateActiveTax(ctx context.Context, taxType string, targetDate time.Time) (decimal.Decimal, error)
}

type taxService struct {
	taxRuleRepo repository.TaxRuleRepository
	auditRepo   repository.AuditRepository
}

func NewTaxService(taxRuleRepo repository.TaxRuleRepository, auditRepo repository.AuditRepository) TaxService {
	return &taxService{taxRuleRepo: taxRuleRepo, auditRepo: auditRepo}
}

// --- Implementation ---

func (s *taxService) GetTaxRules(ctx context.Context, search string, page, limit int) ([]TaxRuleResponse, int64, error) {
	rules, total, err := s.taxRuleRepo.List(ctx, search, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch tax rules: %w", err)
	}

	res := make([]TaxRuleResponse, 0, len(rules))
	for _, r := range rules {
		res = append(res, toTaxRuleResponse(r))
	}

	return res, total, nil
}

func (s *taxService) CreateTaxRule(ctx context.Context, req CreateTaxRuleRequest, userID string) (TaxRuleResponse, error) {
	rate, effectiveFrom, effectiveTo, err := parseTaxRuleFields(req.Rate, req.EffectiveFrom, req.EffectiveTo)
	if err != nil {
		return TaxRuleResponse{}, err
	}

	// Validate overlap
	count, err := s.taxRuleRepo.FindOverlapping(ctx, req.TaxType, effectiveFrom, effectiveTo, nil)
	if err != nil {
		return TaxRuleResponse{}, fmt.Errorf("failed to check overlap: %w", err)
	}
	if count > 0 {
		return TaxRuleResponse{}, fmt.Errorf("a tax rule for '%s' already exists with overlapping effective dates", req.TaxType)
	}

	rule := model.TaxRule{
		TaxType:       req.TaxType,
		Rate:          rate,
		EffectiveFrom: effectiveFrom,
		EffectiveTo:   effectiveTo,
		Description:   req.Description,
	}

	if err := s.taxRuleRepo.Create(ctx, &rule); err != nil {
		return TaxRuleResponse{}, fmt.Errorf("failed to create tax rule: %w", err)
	}

	s.writeAuditLog(ctx, userID, model.ActionCreateTaxRule, rule.ID.String(), req.TaxType+" "+rate.StringFixed(4), req)

	return toTaxRuleResponse(rule), nil
}

func (s *taxService) UpdateTaxRule(ctx context.Context, id string, req UpdateTaxRuleRequest, userID string) (TaxRuleResponse, error) {
	ruleID, err := uuid.Parse(id)
	if err != nil {
		return TaxRuleResponse{}, fmt.Errorf("invalid tax rule id: %w", err)
	}

	rule, err := s.taxRuleRepo.FindByID(ctx, ruleID)
	if err != nil {
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
	count, err := s.taxRuleRepo.FindOverlapping(ctx, req.TaxType, effectiveFrom, effectiveTo, &ruleID)
	if err != nil {
		return TaxRuleResponse{}, fmt.Errorf("failed to check overlap: %w", err)
	}
	if count > 0 {
		return TaxRuleResponse{}, fmt.Errorf("a tax rule for '%s' already exists with overlapping effective dates", req.TaxType)
	}

	rule.TaxType = req.TaxType
	rule.Rate = rate
	rule.EffectiveFrom = effectiveFrom
	rule.EffectiveTo = effectiveTo
	rule.Description = req.Description

	if err := s.taxRuleRepo.Update(ctx, rule); err != nil {
		return TaxRuleResponse{}, fmt.Errorf("failed to update tax rule: %w", err)
	}

	s.writeAuditLog(ctx, userID, model.ActionUpdateTaxRule, rule.ID.String(), req.TaxType+" "+rate.StringFixed(4), req)

	return toTaxRuleResponse(*rule), nil
}

func (s *taxService) DeleteTaxRule(ctx context.Context, id string, userID string) error {
	ruleID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid tax rule id: %w", err)
	}

	rule, err := s.taxRuleRepo.FindByID(ctx, ruleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("tax rule not found")
		}
		return fmt.Errorf("failed to fetch tax rule: %w", err)
	}

	if err := s.taxRuleRepo.Delete(ctx, ruleID); err != nil {
		return fmt.Errorf("failed to delete tax rule: %w", err)
	}

	s.writeAuditLog(ctx, userID, model.ActionDeleteTaxRule, rule.ID.String(), rule.TaxType+" "+rule.Rate.StringFixed(4), map[string]string{"deleted_id": id})

	return nil
}

func (s *taxService) GetActiveTaxRate(ctx context.Context, taxType string) (*ActiveTaxRateResponse, error) {
	now := time.Now()
	rule, err := s.taxRuleRepo.FindActiveByType(ctx, taxType, now)
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

func (s *taxService) CalculateActiveTax(ctx context.Context, taxType string, targetDate time.Time) (decimal.Decimal, error) {
	rule, err := s.taxRuleRepo.FindActiveByType(ctx, taxType, targetDate)
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

	// Best-effort audit log
	_ = s.auditRepo.Log(ctx, &log)
}

// Ensure uuid import is used in DTO context (compiler safeguard)
var _ = uuid.New
