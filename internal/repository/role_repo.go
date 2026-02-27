package repository

import (
	"context"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RoleRepository interface {
	Create(ctx context.Context, role *model.Role) error
	Update(ctx context.Context, role *model.Role) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Role, error)
	FindByIDWithPermissions(ctx context.Context, id uuid.UUID) (*model.Role, error)
	FindByName(ctx context.Context, name string) (*model.Role, error)
	ListAll(ctx context.Context) ([]model.Role, error)
	ListPermissions(ctx context.Context) ([]model.Permission, error)
	UpdatePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	GetPermissionsByRoleName(ctx context.Context, roleName string) ([]string, error)
	FindOrCreatePermission(ctx context.Context, perm *model.Permission) error
	AssociatePermissions(ctx context.Context, roleID uuid.UUID, permIDs []uuid.UUID) error
}

type roleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) RoleRepository {
	return &roleRepository{db: db}
}

func (r *roleRepository) Create(ctx context.Context, role *model.Role) error {
	return GetDB(ctx, r.db).Create(role).Error
}

func (r *roleRepository) Update(ctx context.Context, role *model.Role) error {
	return GetDB(ctx, r.db).Save(role).Error
}

func (r *roleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return GetDB(ctx, r.db).Where("id = ?", id).Delete(&model.Role{}).Error
}

func (r *roleRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	var role model.Role
	if err := GetDB(ctx, r.db).First(&role, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) FindByIDWithPermissions(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	var role model.Role
	if err := GetDB(ctx, r.db).Preload("Permissions").First(&role, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) FindByName(ctx context.Context, name string) (*model.Role, error) {
	var role model.Role
	if err := GetDB(ctx, r.db).Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepository) ListAll(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	if err := GetDB(ctx, r.db).Preload("Permissions").Order("created_at asc").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

func (r *roleRepository) ListPermissions(ctx context.Context) ([]model.Permission, error) {
	var perms []model.Permission
	if err := GetDB(ctx, r.db).Order("\"group\" asc, name asc").Find(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

func (r *roleRepository) UpdatePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	db := GetDB(ctx, r.db)
	var role model.Role
	if err := db.First(&role, "id = ?", roleID).Error; err != nil {
		return err
	}

	var perms []model.Permission
	if err := db.Where("id IN ?", permissionIDs).Find(&perms).Error; err != nil {
		return err
	}

	return db.Model(&role).Association("Permissions").Replace(perms)
}

func (r *roleRepository) GetPermissionsByRoleName(ctx context.Context, roleName string) ([]string, error) {
	var role model.Role
	if err := GetDB(ctx, r.db).Preload("Permissions").Where("name = ?", roleName).First(&role).Error; err != nil {
		return nil, err
	}

	codes := make([]string, 0, len(role.Permissions))
	for _, p := range role.Permissions {
		codes = append(codes, p.Code)
	}
	return codes, nil
}

func (r *roleRepository) FindOrCreatePermission(ctx context.Context, perm *model.Permission) error {
	return GetDB(ctx, r.db).
		Where("code = ?", perm.Code).
		FirstOrCreate(perm).Error
}

func (r *roleRepository) AssociatePermissions(ctx context.Context, roleID uuid.UUID, permIDs []uuid.UUID) error {
	db := GetDB(ctx, r.db)
	var role model.Role
	if err := db.First(&role, "id = ?", roleID).Error; err != nil {
		return err
	}

	var perms []model.Permission
	if err := db.Where("id IN ?", permIDs).Find(&perms).Error; err != nil {
		return err
	}

	return db.Model(&role).Association("Permissions").Append(perms)
}
