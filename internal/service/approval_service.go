package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"backend/internal/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- DTOs ---

type CreateApprovalRequestDTO struct {
	RequestType string `json:"request_type" binding:"required,oneof=CREATE_ORDER CREATE_PRODUCT CREATE_EXPENSE"`
	ReferenceID string `json:"reference_id" binding:"required"`
	RequestData string `json:"request_data" binding:"required"` // JSON snapshot
	RequestedBy string `json:"requested_by"`
}

type ApprovalFilter struct {
	Status string // PENDING, APPROVED, REJECTED or empty for all
	Page   int
	Limit  int
}

type RejectRequestDTO struct {
	Reason string `json:"reason"`
}

type ApprovalRequestResponse struct {
	ID              string  `json:"id"`
	RequestType     string  `json:"request_type"`
	ReferenceID     string  `json:"reference_id"`
	RequestData     string  `json:"request_data"`
	Status          string  `json:"status"`
	RequestedBy     *string `json:"requested_by"`
	RequesterName   string  `json:"requester_name"`
	ApprovedBy      *string `json:"approved_by"`
	ApproverName    string  `json:"approver_name"`
	ApprovedAt      *string `json:"approved_at"`
	RejectionReason string  `json:"rejection_reason"`
	CreatedAt       string  `json:"created_at"`
}

// --- Interface ---

type ApprovalService interface {
	CreateApprovalRequest(ctx context.Context, req CreateApprovalRequestDTO) (ApprovalRequestResponse, error)
	ListApprovalRequests(ctx context.Context, filter ApprovalFilter) ([]ApprovalRequestResponse, int64, error)
	ApproveRequest(ctx context.Context, id string, userID string) (ApprovalRequestResponse, error)
	RejectRequest(ctx context.Context, id string, userID string, reason string) (ApprovalRequestResponse, error)
}

type approvalService struct {
	db  *gorm.DB
	hub interface{ GetBroadcast() chan []byte } // optional websocket hub
}

func NewApprovalService(db *gorm.DB) ApprovalService {
	return &approvalService{db: db}
}

// --- Implementation ---

func (s *approvalService) CreateApprovalRequest(ctx context.Context, req CreateApprovalRequestDTO) (ApprovalRequestResponse, error) {
	refID, err := uuid.Parse(req.ReferenceID)
	if err != nil {
		return ApprovalRequestResponse{}, fmt.Errorf("invalid reference_id: %w", err)
	}

	var requesterID *uuid.UUID
	if req.RequestedBy != "" {
		parsed, parseErr := uuid.Parse(req.RequestedBy)
		if parseErr == nil {
			requesterID = &parsed
		}
	}

	approval := model.ApprovalRequest{
		RequestType: req.RequestType,
		ReferenceID: refID,
		RequestData: req.RequestData,
		Status:      model.ApprovalPending,
		RequestedBy: requesterID,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if createErr := tx.Create(&approval).Error; createErr != nil {
			return fmt.Errorf("failed to create approval request: %w", createErr)
		}

		// Audit log
		details, _ := json.Marshal(map[string]interface{}{
			"request_type": req.RequestType,
			"reference_id": req.ReferenceID,
		})
		audit := model.AuditLog{
			UserID:     requesterID,
			Action:     model.ActionCreateApprovalRequest,
			EntityID:   approval.ID.String(),
			EntityName: req.RequestType,
			Details:    string(details),
		}
		if auditErr := tx.Create(&audit).Error; auditErr != nil {
			return fmt.Errorf("failed to write audit log: %w", auditErr)
		}

		return nil
	})

	if err != nil {
		return ApprovalRequestResponse{}, err
	}

	// Reload with relations
	if loadErr := s.db.WithContext(ctx).Preload("Requester").First(&approval, "id = ?", approval.ID).Error; loadErr != nil {
		return ApprovalRequestResponse{}, fmt.Errorf("failed to reload approval request: %w", loadErr)
	}

	return toApprovalResponse(approval), nil
}

func (s *approvalService) ListApprovalRequests(ctx context.Context, filter ApprovalFilter) ([]ApprovalRequestResponse, int64, error) {
	var total int64
	query := s.db.WithContext(ctx).Model(&model.ApprovalRequest{})

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count approval requests: %w", err)
	}

	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}

	offset := (filter.Page - 1) * filter.Limit
	var approvals []model.ApprovalRequest
	fetchQuery := s.db.WithContext(ctx).
		Preload("Requester").
		Preload("Approver")
	if filter.Status != "" {
		fetchQuery = fetchQuery.Where("status = ?", filter.Status)
	}
	if err := fetchQuery.
		Order("created_at DESC").
		Offset(offset).
		Limit(filter.Limit).
		Find(&approvals).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch approval requests: %w", err)
	}

	result := make([]ApprovalRequestResponse, 0, len(approvals))
	for _, a := range approvals {
		result = append(result, toApprovalResponse(a))
	}

	return result, total, nil
}

func (s *approvalService) ApproveRequest(ctx context.Context, id string, userID string) (ApprovalRequestResponse, error) {
	approvalID, err := uuid.Parse(id)
	if err != nil {
		return ApprovalRequestResponse{}, fmt.Errorf("invalid approval request id: %w", err)
	}

	approverID, err := uuid.Parse(userID)
	if err != nil {
		return ApprovalRequestResponse{}, fmt.Errorf("invalid user id: %w", err)
	}

	var approval model.ApprovalRequest
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if findErr := tx.First(&approval, "id = ?", approvalID).Error; findErr != nil {
			return fmt.Errorf("approval request not found: %w", findErr)
		}

		if approval.Status != model.ApprovalPending {
			return fmt.Errorf("approval request is already %s", approval.Status)
		}

		now := time.Now()
		approval.Status = model.ApprovalApproved
		approval.ApprovedBy = &approverID
		approval.ApprovedAt = &now

		if saveErr := tx.Save(&approval).Error; saveErr != nil {
			return fmt.Errorf("failed to update approval request: %w", saveErr)
		}

		// Execute post-approval actions based on request type
		if execErr := s.executeApproval(ctx, tx, approval, &approverID); execErr != nil {
			return fmt.Errorf("failed to execute approval actions: %w", execErr)
		}

		// Audit log - approval
		details, _ := json.Marshal(map[string]interface{}{
			"request_type": approval.RequestType,
			"reference_id": approval.ReferenceID.String(),
		})
		audit := model.AuditLog{
			UserID:     &approverID,
			Action:     model.ActionApproveRequest,
			EntityID:   approval.ID.String(),
			EntityName: approval.RequestType,
			Details:    string(details),
		}
		if auditErr := tx.Create(&audit).Error; auditErr != nil {
			return fmt.Errorf("failed to write audit log: %w", auditErr)
		}

		return nil
	})

	if err != nil {
		return ApprovalRequestResponse{}, err
	}

	// Reload with relations
	if loadErr := s.db.WithContext(ctx).Preload("Requester").Preload("Approver").First(&approval, "id = ?", approval.ID).Error; loadErr != nil {
		return ApprovalRequestResponse{}, fmt.Errorf("failed to reload approval request: %w", loadErr)
	}

	return toApprovalResponse(approval), nil
}

func (s *approvalService) RejectRequest(ctx context.Context, id string, userID string, reason string) (ApprovalRequestResponse, error) {
	approvalID, err := uuid.Parse(id)
	if err != nil {
		return ApprovalRequestResponse{}, fmt.Errorf("invalid approval request id: %w", err)
	}

	approverID, err := uuid.Parse(userID)
	if err != nil {
		return ApprovalRequestResponse{}, fmt.Errorf("invalid user id: %w", err)
	}

	var approval model.ApprovalRequest
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if findErr := tx.First(&approval, "id = ?", approvalID).Error; findErr != nil {
			return fmt.Errorf("approval request not found: %w", findErr)
		}

		if approval.Status != model.ApprovalPending {
			return fmt.Errorf("approval request is already %s", approval.Status)
		}

		now := time.Now()
		approval.Status = model.ApprovalRejected
		approval.ApprovedBy = &approverID
		approval.ApprovedAt = &now
		approval.RejectionReason = reason

		if saveErr := tx.Save(&approval).Error; saveErr != nil {
			return fmt.Errorf("failed to update approval request: %w", saveErr)
		}

		// If rejecting a CREATE_ORDER, update the order status to REJECTED
		if approval.RequestType == model.ApprovalReqTypeCreateOrder {
			if updateErr := tx.Model(&model.Order{}).Where("id = ?", approval.ReferenceID).
				Update("status", model.OrderStatusRejected).Error; updateErr != nil {
				return fmt.Errorf("failed to update order status: %w", updateErr)
			}
		}

		// Audit log - rejection
		details, _ := json.Marshal(map[string]interface{}{
			"request_type": approval.RequestType,
			"reference_id": approval.ReferenceID.String(),
			"reason":       reason,
		})
		audit := model.AuditLog{
			UserID:     &approverID,
			Action:     model.ActionRejectRequest,
			EntityID:   approval.ID.String(),
			EntityName: approval.RequestType,
			Details:    string(details),
		}
		if auditErr := tx.Create(&audit).Error; auditErr != nil {
			return fmt.Errorf("failed to write audit log: %w", auditErr)
		}

		return nil
	})

	if err != nil {
		return ApprovalRequestResponse{}, err
	}

	// Reload
	if loadErr := s.db.WithContext(ctx).Preload("Requester").Preload("Approver").First(&approval, "id = ?", approval.ID).Error; loadErr != nil {
		return ApprovalRequestResponse{}, fmt.Errorf("failed to reload approval request: %w", loadErr)
	}

	return toApprovalResponse(approval), nil
}

// executeApproval performs the side effects of approving a request:
// For orders: update stock, create inventory transactions, create invoice
// For expenses: create invoice
// For products: no additional action needed
func (s *approvalService) executeApproval(ctx context.Context, tx *gorm.DB, approval model.ApprovalRequest, approverID *uuid.UUID) error {
	switch approval.RequestType {
	case model.ApprovalReqTypeCreateOrder:
		return s.executeOrderApproval(ctx, tx, approval, approverID)
	case model.ApprovalReqTypeCreateExpense:
		return s.executeExpenseApproval(ctx, tx, approval, approverID)
	case model.ApprovalReqTypeCreateProduct:
		// Products are created immediately — approval is just a confirmation
		return nil
	default:
		return fmt.Errorf("unknown request type: %s", approval.RequestType)
	}
}

// executeOrderApproval handles post-approval for orders:
// 1. Load order + items
// 2. Update stock per product (with row locking)
// 3. Create inventory transactions
// 4. Update order status -> COMPLETED
// 5. Create invoice
func (s *approvalService) executeOrderApproval(ctx context.Context, tx *gorm.DB, approval model.ApprovalRequest, approverID *uuid.UUID) error {
	var order model.Order
	if err := tx.Preload("Items").First(&order, "id = ?", approval.ReferenceID).Error; err != nil {
		return fmt.Errorf("order not found: %w", err)
	}

	// Parse request data for tax info
	var reqData struct {
		TaxRuleID string `json:"tax_rule_id"`
		SideFees  string `json:"side_fees"`
	}
	json.Unmarshal([]byte(approval.RequestData), &reqData)

	// Process each order item — update stock + create inventory transactions
	for _, item := range order.Items {
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", item.ProductID).First(&product).Error; err != nil {
			return fmt.Errorf("product not found: %s: %w", item.ProductID, err)
		}

		// Validate export capacity
		if order.Type == model.OrderTypeExport && product.CurrentStock < item.Quantity {
			return fmt.Errorf("insufficient stock for product %s (current: %d, requested: %d)",
				product.Name, product.CurrentStock, item.Quantity)
		}

		modifier := 1
		if order.Type == model.OrderTypeExport {
			modifier = -1
		}

		quantityChanged := item.Quantity * modifier
		stockAfter := product.CurrentStock + quantityChanged

		// Update product stock
		if err := tx.Model(&product).Update("current_stock", stockAfter).Error; err != nil {
			return fmt.Errorf("failed to update stock for product %s: %w", product.Name, err)
		}

		// Create inventory transaction
		txType := model.TxTypeIn
		if order.Type == model.OrderTypeExport {
			txType = model.TxTypeOut
		}

		invTx := model.InventoryTransaction{
			ProductID:       product.ID,
			OrderID:         &order.ID,
			TransactionType: txType,
			QuantityChanged: quantityChanged,
			StockAfter:      stockAfter,
		}
		if err := tx.Create(&invTx).Error; err != nil {
			return fmt.Errorf("failed to record inventory transaction: %w", err)
		}
	}

	// Update order status to COMPLETED
	if err := tx.Model(&order).Update("status", model.OrderStatusCompleted).Error; err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	// Create invoice
	subtotal := decimal.Zero
	for _, item := range order.Items {
		subtotal = subtotal.Add(decimal.NewFromFloat(item.UnitPrice).Mul(decimal.NewFromInt(int64(item.Quantity))))
	}

	sideFees := decimal.Zero
	if reqData.SideFees != "" {
		if parsed, parseErr := decimal.NewFromString(reqData.SideFees); parseErr == nil {
			sideFees = parsed
		}
	}

	taxAmount := decimal.Zero
	var taxRuleID *uuid.UUID
	if reqData.TaxRuleID != "" {
		if parsed, parseErr := uuid.Parse(reqData.TaxRuleID); parseErr == nil {
			taxRuleID = &parsed
			var taxRule model.TaxRule
			if err := tx.First(&taxRule, "id = ?", parsed).Error; err == nil {
				taxAmount = subtotal.Mul(taxRule.Rate)
			}
		}
	}

	totalAmount := subtotal.Add(taxAmount).Add(sideFees)

	invoiceNo, err := s.generateInvoiceNo(tx)
	if err != nil {
		return fmt.Errorf("failed to generate invoice number: %w", err)
	}

	refType := model.RefTypeOrderImport
	if order.Type == model.OrderTypeExport {
		refType = model.RefTypeOrderExport
	}

	invoice := model.Invoice{
		InvoiceNo:      invoiceNo,
		ReferenceType:  refType,
		ReferenceID:    order.ID,
		TaxRuleID:      taxRuleID,
		Subtotal:       subtotal,
		TaxAmount:      taxAmount,
		SideFees:       sideFees,
		TotalAmount:    totalAmount,
		ApprovalStatus: model.ApprovalApproved,
		ApprovedBy:     approverID,
		ApprovedAt:     approval.ApprovedAt,
		Note:           order.Note,
	}
	if err := tx.Create(&invoice).Error; err != nil {
		return fmt.Errorf("failed to create invoice: %w", err)
	}

	// Audit log for invoice creation
	invoiceDetails, _ := json.Marshal(map[string]interface{}{
		"invoice_no": invoiceNo,
		"total":      totalAmount.StringFixed(4),
		"order_code": order.OrderCode,
		"order_type": order.Type,
	})
	auditInvoice := model.AuditLog{
		UserID:     approverID,
		Action:     model.ActionCreateInvoiceFromApproval,
		EntityID:   invoice.ID.String(),
		EntityName: invoiceNo,
		Details:    string(invoiceDetails),
	}
	if err := tx.Create(&auditInvoice).Error; err != nil {
		return fmt.Errorf("failed to write invoice audit log: %w", err)
	}

	return nil
}

// executeExpenseApproval handles post-approval for expenses — creates an invoice
func (s *approvalService) executeExpenseApproval(ctx context.Context, tx *gorm.DB, approval model.ApprovalRequest, approverID *uuid.UUID) error {
	var expense model.Expense
	if err := tx.First(&expense, "id = ?", approval.ReferenceID).Error; err != nil {
		return fmt.Errorf("expense not found: %w", err)
	}

	invoiceNo, err := s.generateInvoiceNo(tx)
	if err != nil {
		return fmt.Errorf("failed to generate invoice number: %w", err)
	}

	subtotal := expense.ConvertedAmountUSD
	taxAmount := expense.VATAmount.Add(expense.FCTAmount)
	totalAmount := subtotal.Add(taxAmount)

	invoice := model.Invoice{
		InvoiceNo:      invoiceNo,
		ReferenceType:  model.RefTypeExpense,
		ReferenceID:    expense.ID,
		Subtotal:       subtotal,
		TaxAmount:      taxAmount,
		SideFees:       decimal.Zero,
		TotalAmount:    totalAmount,
		ApprovalStatus: model.ApprovalApproved,
		ApprovedBy:     approverID,
		ApprovedAt:     approval.ApprovedAt,
		Note:           expense.Description,
	}

	if err := tx.Create(&invoice).Error; err != nil {
		return fmt.Errorf("failed to create invoice from expense: %w", err)
	}

	// Audit log for invoice creation
	invoiceDetails, _ := json.Marshal(map[string]interface{}{
		"invoice_no": invoiceNo,
		"total":      totalAmount.StringFixed(4),
		"expense_id": expense.ID.String(),
	})
	auditInvoice := model.AuditLog{
		UserID:     approverID,
		Action:     model.ActionCreateInvoiceFromApproval,
		EntityID:   invoice.ID.String(),
		EntityName: invoiceNo,
		Details:    string(invoiceDetails),
	}
	if err := tx.Create(&auditInvoice).Error; err != nil {
		return fmt.Errorf("failed to write invoice audit log: %w", err)
	}

	return nil
}

func (s *approvalService) generateInvoiceNo(tx *gorm.DB) (string, error) {
	today := time.Now().Format("20060102")
	prefix := "INV-" + today + "-"

	// Use advisory lock to prevent concurrent duplicate invoice numbers
	tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", prefix)

	var count int64
	if err := tx.Model(&model.Invoice{}).
		Where("invoice_no LIKE ?", prefix+"%").
		Count(&count).Error; err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%05d", prefix, count+1), nil
}

// --- Helpers ---

func toApprovalResponse(a model.ApprovalRequest) ApprovalRequestResponse {
	resp := ApprovalRequestResponse{
		ID:              a.ID.String(),
		RequestType:     a.RequestType,
		ReferenceID:     a.ReferenceID.String(),
		RequestData:     a.RequestData,
		Status:          a.Status,
		RejectionReason: a.RejectionReason,
		CreatedAt:       a.CreatedAt.Format(time.RFC3339),
	}

	if a.RequestedBy != nil {
		s := a.RequestedBy.String()
		resp.RequestedBy = &s
	}
	if a.Requester != nil {
		resp.RequesterName = a.Requester.Username
	}
	if a.ApprovedBy != nil {
		s := a.ApprovedBy.String()
		resp.ApprovedBy = &s
	}
	if a.Approver != nil {
		resp.ApproverName = a.Approver.Username
	}
	if a.ApprovedAt != nil {
		s := a.ApprovedAt.Format(time.RFC3339)
		resp.ApprovedAt = &s
	}

	return resp
}
