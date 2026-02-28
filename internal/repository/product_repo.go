package repository

import (
	"context"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProductRepository interface {
	Create(ctx context.Context, product *model.Product) error
	Update(ctx context.Context, product *model.Product) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error)
	FindBySKU(ctx context.Context, sku string) (*model.Product, error)
	List(ctx context.Context, page, limit int, search string) ([]model.Product, int64, error)
	UpdateStock(ctx context.Context, id uuid.UUID, stock int) error
	FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*model.Product, error)
}

type productRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) ProductRepository {
	return &productRepository{db: db}
}

func (r *productRepository) Create(ctx context.Context, product *model.Product) error {
	return GetDB(ctx, r.db).Create(product).Error
}

func (r *productRepository) Update(ctx context.Context, product *model.Product) error {
	return GetDB(ctx, r.db).Save(product).Error
}

func (r *productRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return GetDB(ctx, r.db).Where("id = ?", id).Delete(&model.Product{}).Error
}

func (r *productRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	var product model.Product
	if err := GetDB(ctx, r.db).First(&product, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *productRepository) FindBySKU(ctx context.Context, sku string) (*model.Product, error) {
	var product model.Product
	if err := GetDB(ctx, r.db).Where("sku = ?", sku).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}

func (r *productRepository) List(ctx context.Context, page, limit int, search string) ([]model.Product, int64, error) {
	var products []model.Product
	var total int64

	db := GetDB(ctx, r.db).Model(&model.Product{})
	if search != "" {
		db = db.Where("name ILIKE ?", "%"+search+"%")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := db.Order("created_at desc").Offset(offset).Limit(limit).Find(&products).Error; err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

func (r *productRepository) UpdateStock(ctx context.Context, id uuid.UUID, stock int) error {
	return GetDB(ctx, r.db).Model(&model.Product{}).Where("id = ?", id).Update("current_stock", stock).Error
}

func (r *productRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	var product model.Product
	if err := GetDB(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).First(&product).Error; err != nil {
		return nil, err
	}
	return &product, nil
}
