package handler

import (
	"backend/internal/middleware"
	"backend/internal/service"
	"backend/pkg/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService service.UserService
}

// NewUserHandler sets up the routing dependencies for User endpoints
func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// RegisterRoutes binds the endpoints to the gin Engine or RouterGroup
func (h *UserHandler) RegisterRoutes(router *gin.RouterGroup) {
	// Public routes
	router.POST("/login", h.Login)

	// Protected users routes
	users := router.Group("/users")
	users.Use(middleware.RequireRole("admin"))
	{
		users.POST("", h.CreateUser)
		users.GET("", h.ListUsers)
		users.GET("/:id", h.GetUserByID)
		users.PUT("/:id", h.UpdateUser)
		users.DELETE("/:id", h.DeleteUser)
	}
}

// CreateUser handles POST /users requests mapping
// @Summary      Create a new user
// @Description  Creates a new user validating constraints and hashing password
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload  body      service.CreateUserRequest  true  "Create User Payload"
// @Success      201      {object}  response.Response{data=service.UserResponse}
// @Failure      400      {object}  response.Response
// @Router       /users [post]
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req service.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	user, err := h.userService.CreateUser(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, user))
}

// Login handles POST /login to authenticate and return a JWT token
// @Summary      Login user
// @Description  Authenticates a user by email and password, returning a JWT token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload  body      service.LoginUserRequest   true  "Login Credentials"
// @Success      200      {object}  response.Response{data=service.TokenResponse}
// @Failure      400      {object}  response.Response
// @Failure      401      {object}  response.Response
// @Router       /login [post]
func (h *UserHandler) Login(c *gin.Context) {
	var req service.LoginUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload"))
		return
	}

	tokenRes, err := h.userService.Login(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, tokenRes))
}

// ListUsers handles GET /users and extracts pagination controls
// @Summary      List users
// @Description  Retrieves a paginated list of users
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        page   query     int  false  "Page number (default 1)"
// @Param        limit  query     int  false  "Number of items per page (default 10)"
// @Success      200    {object}  response.Response{data=object}
// @Failure      500    {object}  response.Response
// @Router       /users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	users, total, err := h.userService.ListUsers(c.Request.Context(), page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "Failed to fetch users"))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, map[string]interface{}{
		"users": users,
		"total": total,
		"page":  page,
		"limit": limit,
	}))
}

// GetUserByID handles target fetch resolution via GET /users/:id
// @Summary      Get user by ID
// @Description  Fetch a single user's detail by their UUID
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  response.Response{data=service.UserResponse}
// @Failure      404  {object}  response.Response
// @Router       /users/{id} [get]
func (h *UserHandler) GetUserByID(c *gin.Context) {
	id := c.Param("id")

	user, err := h.userService.GetUserByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, response.Error(http.StatusNotFound, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, user))
}

// UpdateUser handles target mutative changes via PUT /users/:id
// @Summary      Update user
// @Description  Updates a user's details excluding password
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      string                     true  "User ID"
// @Param        payload  body      service.UpdateUserRequest  true  "Update User Payload"
// @Success      200      {object}  response.Response{data=service.UserResponse}
// @Failure      400      {object}  response.Response
// @Router       /users/{id} [put]
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	var req service.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload"))
		return
	}

	user, err := h.userService.UpdateUser(c.Request.Context(), id, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, user))
}

// DeleteUser orchestrates logical deletion mapping via DELETE /users/:id
// @Summary      Delete user
// @Description  Soft deletes a user by ID
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Router       /users/{id} [delete]
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")

	err := h.userService.DeleteUser(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, "User deleted successfully"))
}
