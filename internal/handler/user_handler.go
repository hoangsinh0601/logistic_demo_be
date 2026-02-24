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
	router.POST("/refresh", h.RefreshToken)
	router.POST("/logout", h.Logout)

	// Me route (authenticated — any valid token)
	router.GET("/me", middleware.RequireRole("admin", "manager", "staff"), h.GetMe)

	// Temp route for admin creation
	router.POST("/temp-admin", h.CreateTempAdmin)

	// Protected users routes
	users := router.Group("/users")
	{
		users.GET("", middleware.RequirePermission("users.read"), h.ListUsers)
		users.GET("/:id", middleware.RequirePermission("users.read"), h.GetUserByID)
		users.POST("", middleware.RequirePermission("users.write"), h.CreateUser)
		users.PUT("/:id", middleware.RequirePermission("users.write"), h.UpdateUser)
		users.DELETE("/:id", middleware.RequirePermission("users.delete"), h.DeleteUser)
	}
}

// CreateTempAdmin creates a temporary admin mapping
// @Summary      Create temporary admin
// @Description  Creates an admin user without requiring authentication. FOR DEVELOPMENT ONLY.
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        payload  body      service.CreateUserRequest  true  "Create Admin Payload"
// @Success      201      {object}  response.Response{data=service.UserResponse}
// @Failure      400      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /temp-admin [post]
func (h *UserHandler) CreateTempAdmin(c *gin.Context) {
	var req service.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload: "+err.Error()))
		return
	}

	req.Role = "admin" // Force admin role
	user, err := h.userService.CreateUser(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}

	c.JSON(http.StatusCreated, response.Success(http.StatusCreated, user))
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

	// Set tokens as HttpOnly cookies
	middleware.SetTokenCookies(c, tokenRes.Token, tokenRes.RefreshToken)

	c.JSON(http.StatusOK, response.Success(http.StatusOK, tokenRes))
}

// GetMe handles GET /me to return current authenticated user based on JWT
// @Summary      Get current user
// @Description  Get the currently authenticated user
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200      {object}  response.Response{data=service.UserResponse}
// @Failure      401      {object}  response.Response
// @Failure      404      {object}  response.Response
// @Router       /me [get]
func (h *UserHandler) GetMe(c *gin.Context) {
	userId, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "User ID not found in context"))
		return
	}

	idStr, ok := userId.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Invalid User ID format"))
		return
	}

	user, err := h.userService.GetUserByID(c.Request.Context(), idStr)
	if err != nil {
		c.JSON(http.StatusNotFound, response.Error(http.StatusNotFound, "User not found"))
		return
	}

	// Fetch permissions for user's role
	perms, _ := middleware.GetPermissionsForRoleFromDB(user.Role)
	if perms == nil {
		perms = []string{}
	}

	c.JSON(http.StatusOK, response.Success(http.StatusOK, map[string]interface{}{
		"id":          user.ID,
		"username":    user.Username,
		"email":       user.Email,
		"role":        user.Role,
		"phone":       user.Phone,
		"permissions": perms,
	}))
}

// RefreshToken handles POST /refresh to issue new access and refresh tokens
// @Summary      Refresh token
// @Description  Issues a new access token and refresh token using a valid refresh token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload  body      service.RefreshTokenRequest   true  "Refresh Token"
// @Success      200      {object}  response.Response{data=service.TokenResponse}
// @Failure      400      {object}  response.Response
// @Failure      401      {object}  response.Response
// @Router       /refresh [post]
func (h *UserHandler) RefreshToken(c *gin.Context) {
	// Try reading refresh_token from cookie first, fallback to body
	refreshToken, cookieErr := c.Cookie("refresh_token")
	var req service.RefreshTokenRequest

	if cookieErr != nil || refreshToken == "" {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "Invalid request payload"))
			return
		}
	} else {
		req = service.RefreshTokenRequest{RefreshToken: refreshToken}
	}

	tokenRes, err := h.userService.RefreshToken(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, err.Error()))
		return
	}

	// Set new tokens as HttpOnly cookies
	middleware.SetTokenCookies(c, tokenRes.Token, tokenRes.RefreshToken)

	c.JSON(http.StatusOK, response.Success(http.StatusOK, tokenRes))
}

// Logout handles POST /logout to clear auth cookies
func (h *UserHandler) Logout(c *gin.Context) {
	middleware.ClearTokenCookies(c)
	c.JSON(http.StatusOK, response.Success(http.StatusOK, "Logged out"))
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
