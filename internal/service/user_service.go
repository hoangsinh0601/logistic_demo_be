package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"os"
	"regexp"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// DTOs for Request validation
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Phone    string `json:"phone" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role" binding:"required"`
}

type UpdateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email" binding:"omitempty,email"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
}

type LoginUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type TokenResponse struct {
	Token string `json:"token"`
}

// DTO for returning User without exposing sensitive data (e.g. password)
type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Role      string    `json:"role"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// UserService defines the interface for business logic related to User
type UserService interface {
	CreateUser(ctx context.Context, req CreateUserRequest) (*UserResponse, error)
	Login(ctx context.Context, req LoginUserRequest) (*TokenResponse, error)
	GetUserByID(ctx context.Context, id string) (*UserResponse, error)
	ListUsers(ctx context.Context, page, limit int) ([]UserResponse, int64, error)
	UpdateUser(ctx context.Context, id string, req UpdateUserRequest) (*UserResponse, error)
	DeleteUser(ctx context.Context, id string) error
}

type userService struct {
	repo repository.UserRepository
}

// NewUserService returns a new instance of UserService
func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

// Helper: check if role is allowed
func validateRole(role string) bool {
	return role == "admin" || role == "quản lý" || role == "nhân viên"
}

// Helper: parse model to standard json API response
func mapToResponse(user *model.User) *UserResponse {
	return &UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		Phone:     user.Phone,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: user.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (s *userService) CreateUser(ctx context.Context, req CreateUserRequest) (*UserResponse, error) {
	if !validateRole(req.Role) {
		return nil, errors.New("invalid role: must be admin, quản lý, or nhân viên")
	}

	// Basic Email format validation fallback
	emailRegex := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
	if !emailRegex.MatchString(req.Email) {
		return nil, errors.New("invalid email format")
	}

	// Double check username/email uniqueness via repo directly
	if _, err := s.repo.GetByUsername(ctx, req.Username); err == nil {
		return nil, errors.New("username already exists")
	}

	if _, err := s.repo.GetByEmail(ctx, req.Email); err == nil {
		return nil, errors.New("email already exists")
	}

	// Hash password automatically
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &model.User{
		Username: req.Username,
		Email:    req.Email,
		Phone:    req.Phone,
		Password: string(hashedPassword),
		Role:     req.Role, // Guaranteed valid by validateRole logic above
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return mapToResponse(user), nil
}

func (s *userService) Login(ctx context.Context, req LoginUserRequest) (*TokenResponse, error) {
	user, err := s.repo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, errors.New("invalid email or password")
	}

	// Generate JWT Token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  user.ID.String(),
		"role": user.Role,
	})

	// Use same fallback strategy as middleware for simplicity here or get from env centrally
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "default_super_secret_key"
	}

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return nil, errors.New("failed to generate token")
	}

	return &TokenResponse{Token: tokenString}, nil
}

func (s *userService) GetUserByID(ctx context.Context, id string) (*UserResponse, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("user not found")
	}
	return mapToResponse(user), nil
}

func (s *userService) ListUsers(ctx context.Context, page, limit int) ([]UserResponse, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}

	users, total, err := s.repo.List(ctx, page, limit)
	if err != nil {
		return nil, 0, err
	}

	var responses []UserResponse
	for _, u := range users {
		responses = append(responses, *mapToResponse(&u))
	}

	return responses, total, nil
}

func (s *userService) UpdateUser(ctx context.Context, id string, req UpdateUserRequest) (*UserResponse, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("user not found")
	}

	if req.Role != "" {
		if !validateRole(req.Role) {
			return nil, errors.New("invalid role: must be admin, quản lý, or nhân viên")
		}
		user.Role = req.Role
	}

	if req.Username != "" && req.Username != user.Username {
		if _, err := s.repo.GetByUsername(ctx, req.Username); err == nil {
			return nil, errors.New("username already exists")
		}
		user.Username = req.Username
	}

	if req.Email != "" && req.Email != user.Email {
		if _, err := s.repo.GetByEmail(ctx, req.Email); err == nil {
			return nil, errors.New("email already exists")
		}
		user.Email = req.Email
	}

	if req.Phone != "" {
		user.Phone = req.Phone
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}

	return mapToResponse(user), nil
}

func (s *userService) DeleteUser(ctx context.Context, id string) error {
	// Let repo handle existence check implicitly or we can check first
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return errors.New("user not found")
	}
	return s.repo.Delete(ctx, id)
}
