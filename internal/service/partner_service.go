package service

import (
	"context"
	"fmt"
	"net/mail"
	"time"

	"backend/internal/model"
	"backend/internal/repository"

	"github.com/google/uuid"
)

// --- Address DTO ---

type AddressPayload struct {
	AddressType string `json:"address_type"`
	FullAddress string `json:"full_address"`
	IsDefault   bool   `json:"is_default"`
}

type AddressResponse struct {
	ID          uuid.UUID `json:"id"`
	PartnerID   uuid.UUID `json:"partner_id"`
	AddressType string    `json:"address_type"`
	FullAddress string    `json:"full_address"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// --- Partner DTOs ---

type CreatePartnerRequest struct {
	Name          string           `json:"name" binding:"required"`
	Type          string           `json:"type" binding:"required"`
	TaxCode       string           `json:"tax_code"`
	CompanyName   string           `json:"company_name"`
	BankAccount   string           `json:"bank_account"`
	ContactPerson string           `json:"contact_person"`
	Phone         string           `json:"phone"`
	Email         string           `json:"email"`
	Addresses     []AddressPayload `json:"addresses"`
}

type UpdatePartnerRequest struct {
	Name          *string           `json:"name"`
	Type          *string           `json:"type"`
	TaxCode       *string           `json:"tax_code"`
	CompanyName   *string           `json:"company_name"`
	BankAccount   *string           `json:"bank_account"`
	ContactPerson *string           `json:"contact_person"`
	Phone         *string           `json:"phone"`
	Email         *string           `json:"email"`
	IsActive      *bool             `json:"is_active"`
	Addresses     *[]AddressPayload `json:"addresses"` // pointer so nil = not sent, [] = clear all
}

type PartnerResponse struct {
	ID            uuid.UUID         `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	TaxCode       string            `json:"tax_code"`
	CompanyName   string            `json:"company_name"`
	BankAccount   string            `json:"bank_account"`
	ContactPerson string            `json:"contact_person"`
	Phone         string            `json:"phone"`
	Email         string            `json:"email"`
	IsActive      bool              `json:"is_active"`
	Addresses     []AddressResponse `json:"addresses"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// --- Interface ---

type PartnerService interface {
	CreatePartner(ctx context.Context, req CreatePartnerRequest) (PartnerResponse, error)
	UpdatePartner(ctx context.Context, id string, req UpdatePartnerRequest) (PartnerResponse, error)
	DeletePartner(ctx context.Context, id string) error
	GetPartners(ctx context.Context, partnerType, search string, page, limit int) ([]PartnerResponse, int64, error)
}

// --- Implementation ---

type partnerService struct {
	partnerRepo repository.PartnerRepository
	txManager   repository.TransactionManager
}

func NewPartnerService(partnerRepo repository.PartnerRepository, txManager repository.TransactionManager) PartnerService {
	return &partnerService{partnerRepo: partnerRepo, txManager: txManager}
}

// --- Validation helpers ---

var validPartnerTypes = map[string]bool{
	model.PartnerTypeCustomer: true,
	model.PartnerTypeSupplier: true,
	model.PartnerTypeBoth:     true,
}

var validAddressTypes = map[string]bool{
	model.AddressTypeBilling:  true,
	model.AddressTypeShipping: true,
	model.AddressTypeOrigin:   true,
}

func validateAddresses(addresses []AddressPayload) error {
	for i, addr := range addresses {
		if !validAddressTypes[addr.AddressType] {
			return fmt.Errorf("addresses[%d]: address_type must be one of: BILLING, SHIPPING, ORIGIN", i)
		}
		if addr.FullAddress == "" {
			return fmt.Errorf("addresses[%d]: full_address is required", i)
		}
	}
	return nil
}

func toAddressModels(partnerID uuid.UUID, payloads []AddressPayload) []model.PartnerAddress {
	addresses := make([]model.PartnerAddress, 0, len(payloads))
	for _, p := range payloads {
		addresses = append(addresses, model.PartnerAddress{
			PartnerID:   partnerID,
			AddressType: p.AddressType,
			FullAddress: p.FullAddress,
			IsDefault:   p.IsDefault,
		})
	}
	return addresses
}

// --- CRUD ---

func (s *partnerService) CreatePartner(ctx context.Context, req CreatePartnerRequest) (PartnerResponse, error) {
	if req.Name == "" {
		return PartnerResponse{}, fmt.Errorf("name is required")
	}
	if !validPartnerTypes[req.Type] {
		return PartnerResponse{}, fmt.Errorf("type must be one of: CUSTOMER, SUPPLIER, BOTH")
	}
	if req.Email != "" {
		if _, err := mail.ParseAddress(req.Email); err != nil {
			return PartnerResponse{}, fmt.Errorf("invalid email format")
		}
	}
	if err := validateAddresses(req.Addresses); err != nil {
		return PartnerResponse{}, err
	}

	partner := &model.Partner{
		Name:          req.Name,
		Type:          req.Type,
		TaxCode:       req.TaxCode,
		CompanyName:   req.CompanyName,
		BankAccount:   req.BankAccount,
		ContactPerson: req.ContactPerson,
		Phone:         req.Phone,
		Email:         req.Email,
		IsActive:      true,
		Addresses:     toAddressModels(uuid.Nil, req.Addresses), // GORM fills PartnerID on cascade create
	}

	// GORM creates partner + addresses in a single Create because of the association
	if err := s.partnerRepo.Create(ctx, partner); err != nil {
		return PartnerResponse{}, fmt.Errorf("failed to create partner: %w", err)
	}

	return toPartnerResponse(*partner), nil
}

func (s *partnerService) UpdatePartner(ctx context.Context, id string, req UpdatePartnerRequest) (PartnerResponse, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return PartnerResponse{}, fmt.Errorf("invalid partner ID")
	}

	partner, err := s.partnerRepo.FindByID(ctx, uid)
	if err != nil {
		return PartnerResponse{}, fmt.Errorf("partner not found: %w", err)
	}

	// Apply field updates
	if req.Name != nil {
		if *req.Name == "" {
			return PartnerResponse{}, fmt.Errorf("name cannot be empty")
		}
		partner.Name = *req.Name
	}
	if req.Type != nil {
		if !validPartnerTypes[*req.Type] {
			return PartnerResponse{}, fmt.Errorf("type must be one of: CUSTOMER, SUPPLIER, BOTH")
		}
		partner.Type = *req.Type
	}
	if req.Email != nil && *req.Email != "" {
		if _, err := mail.ParseAddress(*req.Email); err != nil {
			return PartnerResponse{}, fmt.Errorf("invalid email format")
		}
		partner.Email = *req.Email
	} else if req.Email != nil {
		partner.Email = ""
	}
	if req.TaxCode != nil {
		partner.TaxCode = *req.TaxCode
	}
	if req.CompanyName != nil {
		partner.CompanyName = *req.CompanyName
	}
	if req.BankAccount != nil {
		partner.BankAccount = *req.BankAccount
	}
	if req.ContactPerson != nil {
		partner.ContactPerson = *req.ContactPerson
	}
	if req.Phone != nil {
		partner.Phone = *req.Phone
	}
	if req.IsActive != nil {
		partner.IsActive = *req.IsActive
	}

	// Validate addresses if provided
	if req.Addresses != nil {
		if err := validateAddresses(*req.Addresses); err != nil {
			return PartnerResponse{}, err
		}
	}

	// Run update + address replacement in a transaction
	err = s.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		// Update partner fields
		if err := s.partnerRepo.Update(txCtx, partner); err != nil {
			return fmt.Errorf("failed to update partner: %w", err)
		}

		// Replace addresses if provided (delete-all + re-create strategy)
		if req.Addresses != nil {
			if err := s.partnerRepo.DeleteAddressesByPartnerID(txCtx, uid); err != nil {
				return fmt.Errorf("failed to delete old addresses: %w", err)
			}
			newAddrs := toAddressModels(uid, *req.Addresses)
			if err := s.partnerRepo.CreateAddresses(txCtx, newAddrs); err != nil {
				return fmt.Errorf("failed to create addresses: %w", err)
			}
			partner.Addresses = newAddrs
		}

		return nil
	})
	if err != nil {
		return PartnerResponse{}, err
	}

	return toPartnerResponse(*partner), nil
}

func (s *partnerService) DeletePartner(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid partner ID")
	}
	return s.partnerRepo.Delete(ctx, uid)
}

func (s *partnerService) GetPartners(ctx context.Context, partnerType, search string, page, limit int) ([]PartnerResponse, int64, error) {
	partners, total, err := s.partnerRepo.List(ctx, partnerType, search, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch partners: %w", err)
	}

	res := make([]PartnerResponse, 0, len(partners))
	for _, p := range partners {
		res = append(res, toPartnerResponse(p))
	}

	return res, total, nil
}

// --- Response mappers ---

func toPartnerResponse(p model.Partner) PartnerResponse {
	addresses := make([]AddressResponse, 0, len(p.Addresses))
	for _, a := range p.Addresses {
		addresses = append(addresses, AddressResponse{
			ID:          a.ID,
			PartnerID:   a.PartnerID,
			AddressType: a.AddressType,
			FullAddress: a.FullAddress,
			IsDefault:   a.IsDefault,
			CreatedAt:   a.CreatedAt,
			UpdatedAt:   a.UpdatedAt,
		})
	}

	return PartnerResponse{
		ID:            p.ID,
		Name:          p.Name,
		Type:          p.Type,
		TaxCode:       p.TaxCode,
		CompanyName:   p.CompanyName,
		BankAccount:   p.BankAccount,
		ContactPerson: p.ContactPerson,
		Phone:         p.Phone,
		Email:         p.Email,
		IsActive:      p.IsActive,
		Addresses:     addresses,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}
