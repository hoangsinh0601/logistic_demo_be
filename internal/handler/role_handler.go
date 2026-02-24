package handler

import (
	"net/http"

	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type RoleHandler struct {
	roleService service.RoleService
}

func NewRoleHandler(roleService service.RoleService) *RoleHandler {
	return &RoleHandler{roleService: roleService}
}

func (h *RoleHandler) RegisterRoutes(router *gin.RouterGroup) {
	roles := router.Group("/api/roles")
	roles.Use(middleware.RequirePermission("roles.manage"))
	{
		roles.GET("", h.ListRoles)
		roles.GET("/:id", h.GetRole)
		roles.POST("", h.CreateRole)
		roles.PUT("/:id", h.UpdateRole)
		roles.DELETE("/:id", h.DeleteRole)
		roles.PUT("/:id/permissions", h.UpdateRolePermissions)
	}

	// Permissions list
	perms := router.Group("/api/permissions")
	perms.Use(middleware.RequirePermission("roles.manage"))
	{
		perms.GET("", h.ListPermissions)
	}
}

// ListRoles returns all roles with their permissions
func (h *RoleHandler) ListRoles(c *gin.Context) {
	roles, err := h.roleService.ListRoles(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(http.StatusOK, roles))
}

// GetRole returns a single role by ID
func (h *RoleHandler) GetRole(c *gin.Context) {
	role, err := h.roleService.GetRole(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, response.Error(http.StatusNotFound, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(http.StatusOK, role))
}

// CreateRole creates a new custom role
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req service.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	role, err := h.roleService.CreateRole(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, role))
}

// UpdateRole updates a role's name and description
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	var req service.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	role, err := h.roleService.UpdateRole(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, role))
}

// DeleteRole deletes a non-system role
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	if err := h.roleService.DeleteRole(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, gin.H{"message": "Role deleted successfully"}))
}

// ListPermissions returns all available permissions
func (h *RoleHandler) ListPermissions(c *gin.Context) {
	perms, err := h.roleService.ListPermissions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(http.StatusOK, perms))
}

// UpdateRolePermissions replaces all permissions for a role
func (h *RoleHandler) UpdateRolePermissions(c *gin.Context) {
	var req service.UpdateRolePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	role, err := h.roleService.UpdateRolePermissions(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	// Invalidate cached permissions for this role so /me returns fresh data
	middleware.ClearPermissionCache(role.Name)

	c.JSON(http.StatusOK, response.Success(http.StatusOK, role))
}
