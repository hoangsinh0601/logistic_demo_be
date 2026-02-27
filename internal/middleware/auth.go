package middleware

import (
	"backend/pkg/response"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

func GetJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		if os.Getenv("GIN_MODE") == "release" {
			panic("FATAL: JWT_SECRET environment variable is required in production mode")
		}
		secret = "default_super_secret_key" // Development fallback only — DO NOT use in production
	}
	return []byte(secret)
}

// SetTokenCookies sets access_token and refresh_token as HttpOnly cookies
func SetTokenCookies(c *gin.Context, accessToken, refreshToken string) {
	// Production (cross-origin): SameSiteNoneMode + Secure=true
	// Development (same-site):   SameSiteLaxMode  + Secure=false
	sameSite := http.SameSiteLaxMode
	secure := false
	if os.Getenv("GIN_MODE") == "release" || os.Getenv("RENDER") != "" {
		sameSite = http.SameSiteNoneMode
		secure = true
	}

	c.SetSameSite(sameSite)
	// access_token: 24h, path=/, domain="", secure, HttpOnly
	c.SetCookie("access_token", accessToken, 3600*24, "/", "", secure, true)
	// refresh_token: 7 days, path=/, domain="", secure, HttpOnly
	c.SetCookie("refresh_token", refreshToken, 3600*24*7, "/", "", secure, true)
}

// ClearTokenCookies removes access_token and refresh_token cookies
func ClearTokenCookies(c *gin.Context) {
	sameSite := http.SameSiteLaxMode
	secure := false
	if os.Getenv("GIN_MODE") == "release" || os.Getenv("RENDER") != "" {
		sameSite = http.SameSiteNoneMode
		secure = true
	}

	c.SetSameSite(sameSite)
	c.SetCookie("access_token", "", -1, "/", "", secure, true)
	c.SetCookie("refresh_token", "", -1, "/", "", secure, true)
}

// RequireRole Middleware validates the JWT token and checks if the user's role exists in the allowedRoles list
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try cookie first, fallback to Authorization header
		tokenString, cookieErr := c.Cookie("access_token")
		if cookieErr != nil || tokenString == "" {
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Authorization is missing"))
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Invalid authorization format. Expected 'Bearer <token>'"))
				return
			}
			tokenString = parts[1]
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return GetJWTSecret(), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Invalid token: "+err.Error()))
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Invalid token claims"))
			return
		}

		userRole, ok := claims["role"].(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, response.Error(http.StatusForbidden, "Role not found in token"))
			return
		}

		// Check if userRole is in allowedRoles
		roleAllowed := false
		for _, role := range allowedRoles {
			if userRole == role {
				roleAllowed = true
				break
			}
		}

		if !roleAllowed {
			c.AbortWithStatusJSON(http.StatusForbidden, response.Error(http.StatusForbidden, "Access denied: insufficient permissions"))
			return
		}

		// Set Context values if necessary
		c.Set("userID", claims["sub"])
		c.Set("userRole", userRole)

		c.Next()
	}
}

// --- Permission-based middleware ---

// permCacheEntry stores cached permission codes for a role with TTL
type permCacheEntry struct {
	codes     []string
	expiresAt time.Time
}

var (
	permCache    sync.Map // roleName -> permCacheEntry
	permCacheTTL = 5 * time.Minute
)

// permDB holds the database reference for permission queries — set via InitPermissionMiddleware
var permDB *gorm.DB

// InitPermissionMiddleware sets the DB reference for RequirePermission middleware
func InitPermissionMiddleware(db *gorm.DB) {
	permDB = db
}

// RequirePermission validates the JWT and checks if the user's role has the required permission codes.
// Falls back to RequireRole-style check if role is "admin" (admin always passes).
func RequirePermission(requiredPerms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse JWT (same logic as RequireRole)
		tokenString, cookieErr := c.Cookie("access_token")
		if cookieErr != nil || tokenString == "" {
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Authorization is missing"))
				return
			}
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Invalid authorization format"))
				return
			}
			tokenString = parts[1]
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return GetJWTSecret(), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Invalid token"))
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Error(http.StatusUnauthorized, "Invalid token claims"))
			return
		}

		userRole, ok := claims["role"].(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, response.Error(http.StatusForbidden, "Role not found in token"))
			return
		}

		c.Set("userID", claims["sub"])
		c.Set("userRole", userRole)

		// Get user's permission codes (cached)
		userPerms, err := getPermissionsForRole(userRole)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "Failed to verify permissions"))
			return
		}

		// Check if any required permission is present
		permSet := make(map[string]bool, len(userPerms))
		for _, p := range userPerms {
			permSet[p] = true
		}

		for _, required := range requiredPerms {
			if !permSet[required] {
				c.AbortWithStatusJSON(http.StatusForbidden, response.Error(http.StatusForbidden, "Access denied: missing permission '"+required+"'"))
				return
			}
		}

		c.Next()
	}
}

// getPermissionsForRole returns cached or DB-fetched permission codes for a role name
func getPermissionsForRole(roleName string) ([]string, error) {
	// Check cache
	if entry, ok := permCache.Load(roleName); ok {
		cached := entry.(permCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			return cached.codes, nil
		}
	}

	if permDB == nil {
		return nil, fmt.Errorf("permission middleware not initialized")
	}

	// Query: role → role_permissions → permissions
	var codes []string
	err := permDB.Raw(`
		SELECT p.code FROM permissions p
		INNER JOIN role_permissions rp ON rp.permission_id = p.id
		INNER JOIN roles r ON r.id = rp.role_id
		WHERE r.name = ?
	`, roleName).Pluck("code", &codes).Error

	if err != nil {
		return nil, err
	}

	// Cache result
	permCache.Store(roleName, permCacheEntry{
		codes:     codes,
		expiresAt: time.Now().Add(permCacheTTL),
	})

	return codes, nil
}

// GetPermissionsForRoleFromDB exposes permission fetching for handlers (e.g., /me endpoint)
func GetPermissionsForRoleFromDB(roleName string) ([]string, error) {
	return getPermissionsForRole(roleName)
}

// ClearPermissionCache removes cached permissions for a specific role (or all roles if empty)
func ClearPermissionCache(roleName string) {
	if roleName == "" {
		permCache.Range(func(key, _ interface{}) bool {
			permCache.Delete(key)
			return true
		})
	} else {
		permCache.Delete(roleName)
	}
}
