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
// @Summary      List roles
// @Description  Retrieves all roles with their associated permissions
// @Tags         roles
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  response.Response{data=[]service.RoleResponse}
// @Failure      500  {object}  response.Response
// @Router       /api/roles [get]
func (h *RoleHandler) ListRoles(c *gin.Context) {
	roles, err := h.roleService.ListRoles(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(http.StatusOK, roles))
}

// GetRole returns a single role by ID
// @Summary      Get role
// @Description  Retrieves a single role with permissions by UUID
// @Tags         roles
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Role ID"
// @Success      200  {object}  response.Response{data=service.RoleResponse}
// @Failure      404  {object}  response.Response
// @Router       /api/roles/{id} [get]
func (h *RoleHandler) GetRole(c *gin.Context) {
	role, err := h.roleService.GetRole(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, response.Error(http.StatusNotFound, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(http.StatusOK, role))
}

// CreateRole creates a new custom role
// @Summary      Create role
// @Description  Creates a new custom role with optional permission assignments
// @Tags         roles
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        payload  body      service.CreateRoleRequest  true  "Create Role Payload"
// @Success      201      {object}  response.Response{data=service.RoleResponse}
// @Failure      400      {object}  response.Response
// @Router       /api/roles [post]
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
// @Summary      Update role
// @Description  Updates a role's name and description by ID
// @Tags         roles
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path      string                     true  "Role ID"
// @Param        payload  body      service.UpdateRoleRequest  true  "Update Role Payload"
// @Success      200      {object}  response.Response{data=service.RoleResponse}
// @Failure      400      {object}  response.Response
// @Router       /api/roles/{id} [put]
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
// @Summary      Delete role
// @Description  Deletes a non-system role by ID. System roles cannot be deleted.
// @Tags         roles
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Role ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Router       /api/roles/{id} [delete]
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	if err := h.roleService.DeleteRole(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, gin.H{"message": "Role deleted successfully"}))
}

// ListPermissions returns all available permissions
// @Summary      List permissions
// @Description  Retrieves all available permissions grouped by module
// @Tags         roles
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  response.Response{data=[]service.PermissionResponse}
// @Failure      500  {object}  response.Response
// @Router       /api/permissions [get]
func (h *RoleHandler) ListPermissions(c *gin.Context) {
	perms, err := h.roleService.ListPermissions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(http.StatusOK, perms))
}

// UpdateRolePermissions replaces all permissions for a role
// @Summary      Update role permissions
// @Description  Replaces all permissions for a role by ID
// @Tags         roles
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path      string                                true  "Role ID"
// @Param        payload  body      service.UpdateRolePermissionsRequest  true  "Permission IDs"
// @Success      200      {object}  response.Response{data=service.RoleResponse}
// @Failure      400      {object}  response.Response
// @Router       /api/roles/{id}/permissions [put]
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
