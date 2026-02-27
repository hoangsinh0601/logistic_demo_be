package service

import (
	"context"
	"fmt"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// --- DTOs ---

type CreateRoleRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"` // Permission UUIDs
}

type UpdateRoleRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type UpdateRolePermissionsRequest struct {
	PermissionIDs []string `json:"permission_ids" binding:"required"`
}

type RoleResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	IsSystem    bool                 `json:"is_system"`
	Permissions []PermissionResponse `json:"permissions"`
	CreatedAt   string               `json:"created_at"`
}

type PermissionResponse struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

// --- Interface ---

type RoleService interface {
	ListRoles(ctx context.Context) ([]RoleResponse, error)
	GetRole(ctx context.Context, id string) (*RoleResponse, error)
	CreateRole(ctx context.Context, req CreateRoleRequest) (*RoleResponse, error)
	UpdateRole(ctx context.Context, id string, req UpdateRoleRequest) (*RoleResponse, error)
	DeleteRole(ctx context.Context, id string) error
	ListPermissions(ctx context.Context) ([]PermissionResponse, error)
	UpdateRolePermissions(ctx context.Context, roleID string, req UpdateRolePermissionsRequest) (*RoleResponse, error)
	GetPermissionsByRoleName(ctx context.Context, roleName string) ([]string, error)
	SeedDefaultRolesAndPermissions(ctx context.Context) error
}

type roleService struct {
	db *gorm.DB
}

func NewRoleService(db *gorm.DB) RoleService {
	return &roleService{db: db}
}

// --- Implementation ---

func (s *roleService) ListRoles(ctx context.Context) ([]RoleResponse, error) {
	var roles []model.Role
	if err := s.db.WithContext(ctx).Preload("Permissions").Order("name ASC").Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch roles: %w", err)
	}

	res := make([]RoleResponse, 0, len(roles))
	for _, r := range roles {
		res = append(res, toRoleResponse(r))
	}
	return res, nil
}

func (s *roleService) GetRole(ctx context.Context, id string) (*RoleResponse, error) {
	roleID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid role id: %w", err)
	}

	var role model.Role
	if err := s.db.WithContext(ctx).Preload("Permissions").First(&role, "id = ?", roleID).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	resp := toRoleResponse(role)
	return &resp, nil
}

func (s *roleService) CreateRole(ctx context.Context, req CreateRoleRequest) (*RoleResponse, error) {
	role := model.Role{
		Name:        req.Name,
		Description: req.Description,
		IsSystem:    false,
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&role).Error; err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}

		if len(req.Permissions) > 0 {
			var perms []model.Permission
			permIDs := make([]uuid.UUID, 0, len(req.Permissions))
			for _, pid := range req.Permissions {
				parsed, parseErr := uuid.Parse(pid)
				if parseErr != nil {
					return fmt.Errorf("invalid permission id '%s': %w", pid, parseErr)
				}
				permIDs = append(permIDs, parsed)
			}
			if err := tx.Where("id IN ?", permIDs).Find(&perms).Error; err != nil {
				return fmt.Errorf("failed to fetch permissions: %w", err)
			}
			if err := tx.Model(&role).Association("Permissions").Replace(perms); err != nil {
				return fmt.Errorf("failed to assign permissions: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Reload with permissions
	return s.GetRole(ctx, role.ID.String())
}

func (s *roleService) UpdateRole(ctx context.Context, id string, req UpdateRoleRequest) (*RoleResponse, error) {
	roleID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid role id: %w", err)
	}

	var role model.Role
	if err := s.db.WithContext(ctx).First(&role, "id = ?", roleID).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	role.Name = req.Name
	role.Description = req.Description

	if err := s.db.WithContext(ctx).Save(&role).Error; err != nil {
		return nil, fmt.Errorf("failed to update role: %w", err)
	}

	return s.GetRole(ctx, id)
}

func (s *roleService) DeleteRole(ctx context.Context, id string) error {
	roleID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid role id: %w", err)
	}

	var role model.Role
	if err := s.db.WithContext(ctx).First(&role, "id = ?", roleID).Error; err != nil {
		return fmt.Errorf("role not found: %w", err)
	}

	if role.IsSystem {
		return fmt.Errorf("cannot delete system role '%s'", role.Name)
	}

	// Clear associations before deleting
	if err := s.db.WithContext(ctx).Model(&role).Association("Permissions").Clear(); err != nil {
		return fmt.Errorf("failed to clear permissions: %w", err)
	}

	if err := s.db.WithContext(ctx).Delete(&role).Error; err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	return nil
}

func (s *roleService) ListPermissions(ctx context.Context) ([]PermissionResponse, error) {
	var perms []model.Permission
	if err := s.db.WithContext(ctx).Order("\"group\" ASC, code ASC").Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch permissions: %w", err)
	}

	res := make([]PermissionResponse, 0, len(perms))
	for _, p := range perms {
		res = append(res, toPermissionResponse(p))
	}
	return res, nil
}

func (s *roleService) UpdateRolePermissions(ctx context.Context, roleID string, req UpdateRolePermissionsRequest) (*RoleResponse, error) {
	id, err := uuid.Parse(roleID)
	if err != nil {
		return nil, fmt.Errorf("invalid role id: %w", err)
	}

	var role model.Role
	if err := s.db.WithContext(ctx).First(&role, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	var perms []model.Permission
	if len(req.PermissionIDs) > 0 {
		permIDs := make([]uuid.UUID, 0, len(req.PermissionIDs))
		for _, pid := range req.PermissionIDs {
			parsed, parseErr := uuid.Parse(pid)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid permission id '%s': %w", pid, parseErr)
			}
			permIDs = append(permIDs, parsed)
		}
		if err := s.db.WithContext(ctx).Where("id IN ?", permIDs).Find(&perms).Error; err != nil {
			return nil, fmt.Errorf("failed to fetch permissions: %w", err)
		}
	}

	if err := s.db.WithContext(ctx).Model(&role).Association("Permissions").Replace(perms); err != nil {
		return nil, fmt.Errorf("failed to update permissions: %w", err)
	}

	return s.GetRole(ctx, roleID)
}

func (s *roleService) GetPermissionsByRoleName(ctx context.Context, roleName string) ([]string, error) {
	var role model.Role
	if err := s.db.WithContext(ctx).Preload("Permissions").Where("name = ?", roleName).First(&role).Error; err != nil {
		return nil, fmt.Errorf("role '%s' not found: %w", roleName, err)
	}

	codes := make([]string, 0, len(role.Permissions))
	for _, p := range role.Permissions {
		codes = append(codes, p.Code)
	}
	return codes, nil
}

// SeedDefaultRolesAndPermissions creates the default permissions and roles if not already present
func (s *roleService) SeedDefaultRolesAndPermissions(ctx context.Context) error {
	// Define all permissions
	defaultPermissions := []model.Permission{
		{Code: "dashboard.read", Name: "Xem Dashboard & Thống kê TC", Group: "dashboard"},
		{Code: "inventory.read", Name: "Xem Kho hàng", Group: "inventory"},
		{Code: "inventory.write", Name: "Quản lý Kho hàng", Group: "inventory"},
		{Code: "expenses.read", Name: "Xem Chi phí", Group: "expenses"},
		{Code: "expenses.write", Name: "Tạo Chi phí", Group: "expenses"},
		{Code: "tax_rules.read", Name: "Xem Thuế suất", Group: "tax"},
		{Code: "tax_rules.write", Name: "Quản lý Thuế suất", Group: "tax"},
		{Code: "users.read", Name: "Xem Người dùng", Group: "users"},
		{Code: "users.write", Name: "Quản lý Người dùng", Group: "users"},
		{Code: "users.delete", Name: "Xóa Người dùng", Group: "users"},
		{Code: "audit.read", Name: "Xem Lịch sử hoạt động", Group: "audit"},
		{Code: "roles.manage", Name: "Quản lý Phân quyền", Group: "roles"},
		// Invoice permissions
		{Code: "invoices.read", Name: "Xem Hóa đơn", Group: "invoices"},
		{Code: "invoices.write", Name: "Tạo Hóa đơn", Group: "invoices"},
		// Approval permissions
		{Code: "approvals.read", Name: "Xem Yêu cầu duyệt", Group: "approvals"},
		{Code: "approvals.approve", Name: "Duyệt / Từ chối yêu cầu", Group: "approvals"},
		// Finance
		{Code: "finance.read", Name: "Xem Báo cáo Tài chính", Group: "finance"},
	}

	// Upsert permissions
	for i := range defaultPermissions {
		p := &defaultPermissions[i]
		var existing model.Permission
		result := s.db.WithContext(ctx).Where("code = ?", p.Code).First(&existing)
		if result.Error != nil {
			// Not found, create
			if err := s.db.WithContext(ctx).Create(p).Error; err != nil {
				return fmt.Errorf("failed to seed permission '%s': %w", p.Code, err)
			}
		} else {
			p.ID = existing.ID // Use existing ID
			// Update name/group if changed
			s.db.WithContext(ctx).Exec(
				`UPDATE permissions SET name = ?, "group" = ? WHERE id = ?`,
				p.Name, p.Group, existing.ID,
			)
		}
	}

	// Build permission maps by code for easy lookup
	permByCode := make(map[string]model.Permission)
	for _, p := range defaultPermissions {
		permByCode[p.Code] = p
	}

	allPerms := make([]model.Permission, 0, len(defaultPermissions))
	for _, p := range defaultPermissions {
		allPerms = append(allPerms, p)
	}

	// Define roles with their permissions
	roleDefinitions := map[string]struct {
		Description string
		PermCodes   []string
	}{
		"admin": {
			Description: "Quản trị viên — Toàn quyền hệ thống",
			PermCodes: []string{
				"dashboard.read", "inventory.read", "inventory.write",
				"expenses.read", "expenses.write",
				"tax_rules.read", "tax_rules.write",
				"users.read", "users.write", "users.delete",
				"audit.read", "roles.manage",
				"invoices.read", "invoices.write",
				"approvals.read", "approvals.approve",
				"finance.read",
			},
		},
		"manager": {
			Description: "Quản lý — Duyệt yêu cầu, xem báo cáo, quản lý kho",
			PermCodes: []string{
				"dashboard.read", "inventory.read", "inventory.write",
				"expenses.read", "expenses.write",
				"tax_rules.read", "tax_rules.write",
				"users.read", "users.write",
				"audit.read",
				"invoices.read", "invoices.write",
				"approvals.read", "approvals.approve",
				"finance.read",
			},
		},
		"staff": {
			Description: "Nhân viên — Tạo đơn, xem duyệt, thao tác cơ bản",
			PermCodes: []string{
				"inventory.read", "inventory.write",
				"expenses.read", "expenses.write",
				"tax_rules.read",
				"audit.read",
				"invoices.read",
				"approvals.read",
			},
		},
	}

	for roleName, def := range roleDefinitions {
		var role model.Role
		result := s.db.WithContext(ctx).Where("name = ?", roleName).First(&role)
		if result.Error != nil {
			// Create role
			role = model.Role{
				Name:        roleName,
				Description: def.Description,
				IsSystem:    true,
			}
			if err := s.db.WithContext(ctx).Create(&role).Error; err != nil {
				return fmt.Errorf("failed to seed role '%s': %w", roleName, err)
			}
		}

		// Assign permissions
		perms := make([]model.Permission, 0, len(def.PermCodes))
		for _, code := range def.PermCodes {
			if p, ok := permByCode[code]; ok {
				perms = append(perms, p)
			}
		}
		if err := s.db.WithContext(ctx).Model(&role).Association("Permissions").Replace(perms); err != nil {
			return fmt.Errorf("failed to assign permissions to role '%s': %w", roleName, err)
		}
	}

	return nil
}

// --- Helpers ---

func toRoleResponse(r model.Role) RoleResponse {
	perms := make([]PermissionResponse, 0, len(r.Permissions))
	for _, p := range r.Permissions {
		perms = append(perms, toPermissionResponse(p))
	}

	return RoleResponse{
		ID:          r.ID.String(),
		Name:        r.Name,
		Description: r.Description,
		IsSystem:    r.IsSystem,
		Permissions: perms,
		CreatedAt:   r.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func toPermissionResponse(p model.Permission) PermissionResponse {
	return PermissionResponse{
		ID:    p.ID.String(),
		Code:  p.Code,
		Name:  p.Name,
		Group: p.Group,
	}
}
