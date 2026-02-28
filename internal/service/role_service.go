package service

import (
	"context"
	"errors"
	"fmt"

	"backend/internal/model"
	"backend/internal/repository"

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
	roleRepo  repository.RoleRepository
	txManager repository.TransactionManager
}

func NewRoleService(roleRepo repository.RoleRepository, txManager repository.TransactionManager) RoleService {
	return &roleService{roleRepo: roleRepo, txManager: txManager}
}

// --- Implementation ---

func (s *roleService) ListRoles(ctx context.Context) ([]RoleResponse, error) {
	roles, err := s.roleRepo.ListAll(ctx)
	if err != nil {
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

	role, err := s.roleRepo.FindByIDWithPermissions(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	resp := toRoleResponse(*role)
	return &resp, nil
}

func (s *roleService) CreateRole(ctx context.Context, req CreateRoleRequest) (*RoleResponse, error) {
	role := model.Role{
		Name:        req.Name,
		Description: req.Description,
		IsSystem:    false,
	}

	err := s.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.roleRepo.Create(txCtx, &role); err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}

		if len(req.Permissions) > 0 {
			permIDs := make([]uuid.UUID, 0, len(req.Permissions))
			for _, pid := range req.Permissions {
				parsed, parseErr := uuid.Parse(pid)
				if parseErr != nil {
					return fmt.Errorf("invalid permission id '%s': %w", pid, parseErr)
				}
				permIDs = append(permIDs, parsed)
			}
			if err := s.roleRepo.UpdatePermissions(txCtx, role.ID, permIDs); err != nil {
				return fmt.Errorf("failed to assign permissions: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetRole(ctx, role.ID.String())
}

func (s *roleService) UpdateRole(ctx context.Context, id string, req UpdateRoleRequest) (*RoleResponse, error) {
	roleID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid role id: %w", err)
	}

	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	role.Name = req.Name
	role.Description = req.Description

	if err := s.roleRepo.Update(ctx, role); err != nil {
		return nil, fmt.Errorf("failed to update role: %w", err)
	}

	return s.GetRole(ctx, id)
}

func (s *roleService) DeleteRole(ctx context.Context, id string) error {
	roleID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid role id: %w", err)
	}

	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		return fmt.Errorf("role not found: %w", err)
	}

	if role.IsSystem {
		return fmt.Errorf("cannot delete system role '%s'", role.Name)
	}

	// Clear associations then delete
	if err := s.roleRepo.UpdatePermissions(ctx, roleID, []uuid.UUID{}); err != nil {
		return fmt.Errorf("failed to clear permissions: %w", err)
	}

	if err := s.roleRepo.Delete(ctx, roleID); err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	return nil
}

func (s *roleService) ListPermissions(ctx context.Context) ([]PermissionResponse, error) {
	perms, err := s.roleRepo.ListPermissions(ctx)
	if err != nil {
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

	_, err = s.roleRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	permIDs := make([]uuid.UUID, 0, len(req.PermissionIDs))
	for _, pid := range req.PermissionIDs {
		parsed, parseErr := uuid.Parse(pid)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid permission id '%s': %w", pid, parseErr)
		}
		permIDs = append(permIDs, parsed)
	}

	if err := s.roleRepo.UpdatePermissions(ctx, id, permIDs); err != nil {
		return nil, fmt.Errorf("failed to update permissions: %w", err)
	}

	return s.GetRole(ctx, roleID)
}

func (s *roleService) GetPermissionsByRoleName(ctx context.Context, roleName string) ([]string, error) {
	codes, err := s.roleRepo.GetPermissionsByRoleName(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("role '%s' not found: %w", roleName, err)
	}
	return codes, nil
}

func (s *roleService) SeedDefaultRolesAndPermissions(ctx context.Context) error {
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
		{Code: "invoices.read", Name: "Xem Hóa đơn", Group: "invoices"},
		{Code: "invoices.write", Name: "Tạo Hóa đơn", Group: "invoices"},
		{Code: "approvals.read", Name: "Xem Yêu cầu duyệt", Group: "approvals"},
		{Code: "approvals.approve", Name: "Duyệt / Từ chối yêu cầu", Group: "approvals"},
		{Code: "finance.read", Name: "Xem Báo cáo Tài chính", Group: "finance"},
		{Code: "partners.read", Name: "Xem Đối tác", Group: "partners"},
		{Code: "partners.write", Name: "Quản lý Đối tác", Group: "partners"},
		{Code: "partners.delete", Name: "Xóa Đối tác", Group: "partners"},
	}

	// Upsert permissions
	for i := range defaultPermissions {
		p := &defaultPermissions[i]
		if err := s.roleRepo.FindOrCreatePermission(ctx, p); err != nil {
			return fmt.Errorf("failed to seed permission '%s': %w", p.Code, err)
		}
	}

	permByCode := make(map[string]model.Permission)
	for _, p := range defaultPermissions {
		permByCode[p.Code] = p
	}

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
				"partners.read", "partners.write", "partners.delete",
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
				"partners.read", "partners.write",
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
				"partners.read",
			},
		},
	}

	for roleName, def := range roleDefinitions {
		role, err := s.roleRepo.FindByName(ctx, roleName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				role = &model.Role{
					Name:        roleName,
					Description: def.Description,
					IsSystem:    true,
				}
				if createErr := s.roleRepo.Create(ctx, role); createErr != nil {
					return fmt.Errorf("failed to seed role '%s': %w", roleName, createErr)
				}
			} else {
				return fmt.Errorf("failed to check role '%s': %w", roleName, err)
			}
		}

		// Assign permissions
		permIDs := make([]uuid.UUID, 0, len(def.PermCodes))
		for _, code := range def.PermCodes {
			if p, ok := permByCode[code]; ok {
				permIDs = append(permIDs, p.ID)
			}
		}
		if err := s.roleRepo.UpdatePermissions(ctx, role.ID, permIDs); err != nil {
			return fmt.Errorf("failed to assign permissions to role '%s': %w", roleName, err)
		}
	}

	// Cleanup: remove stale permissions not in the default list
	validCodes := make([]string, 0, len(defaultPermissions))
	for _, p := range defaultPermissions {
		validCodes = append(validCodes, p.Code)
	}
	if err := s.roleRepo.DeletePermissionsNotInCodes(ctx, validCodes); err != nil {
		return fmt.Errorf("failed to cleanup stale permissions: %w", err)
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
