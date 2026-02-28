package repository

import (
	"context"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PartnerRepository interface {
	Create(ctx context.Context, partner *model.Partner) error
	Update(ctx context.Context, partner *model.Partner) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Partner, error)
	List(ctx context.Context, partnerType, search string, page, limit int) ([]model.Partner, int64, error)
	DeleteAddressesByPartnerID(ctx context.Context, partnerID uuid.UUID) error
	CreateAddresses(ctx context.Context, addresses []model.PartnerAddress) error
}

type partnerRepository struct {
	db *gorm.DB
}

func NewPartnerRepository(db *gorm.DB) PartnerRepository {
	return &partnerRepository{db: db}
}

func (r *partnerRepository) Create(ctx context.Context, partner *model.Partner) error {
	return GetDB(ctx, r.db).Create(partner).Error
}

func (r *partnerRepository) Update(ctx context.Context, partner *model.Partner) error {
	return GetDB(ctx, r.db).Save(partner).Error
}

func (r *partnerRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return GetDB(ctx, r.db).Where("id = ?", id).Delete(&model.Partner{}).Error
}

func (r *partnerRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Partner, error) {
	var partner model.Partner
	if err := GetDB(ctx, r.db).Preload("Addresses").First(&partner, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &partner, nil
}

func (r *partnerRepository) List(ctx context.Context, partnerType, search string, page, limit int) ([]model.Partner, int64, error) {
	var partners []model.Partner
	var total int64

	db := GetDB(ctx, r.db)
	query := db.Model(&model.Partner{})

	if partnerType != "" {
		query = query.Where("type = ?", partnerType)
	}
	if search != "" {
		query = query.Where("name ILIKE ? OR company_name ILIKE ? OR phone ILIKE ? OR email ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	fetchQuery := db.Model(&model.Partner{}).Preload("Addresses")
	if partnerType != "" {
		fetchQuery = fetchQuery.Where("type = ?", partnerType)
	}
	if search != "" {
		fetchQuery = fetchQuery.Where("name ILIKE ? OR company_name ILIKE ? OR phone ILIKE ? OR email ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	if err := fetchQuery.Order("created_at DESC").Offset(offset).Limit(limit).Find(&partners).Error; err != nil {
		return nil, 0, err
	}

	return partners, total, nil
}

func (r *partnerRepository) DeleteAddressesByPartnerID(ctx context.Context, partnerID uuid.UUID) error {
	return GetDB(ctx, r.db).Where("partner_id = ?", partnerID).Delete(&model.PartnerAddress{}).Error
}

func (r *partnerRepository) CreateAddresses(ctx context.Context, addresses []model.PartnerAddress) error {
	if len(addresses) == 0 {
		return nil
	}
	return GetDB(ctx, r.db).Create(&addresses).Error
}
