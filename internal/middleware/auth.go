package middleware

import (
	"backend/pkg/response"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func GetJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "default_super_secret_key" // In production, make sure to set JWT_SECRET
	}
	return []byte(secret)
}

// SetTokenCookies sets access_token and refresh_token as HttpOnly cookies
func SetTokenCookies(c *gin.Context, accessToken, refreshToken string) {
	c.SetSameSite(http.SameSiteLaxMode)
	// access_token: 24h, path=/, HttpOnly
	c.SetCookie("access_token", accessToken, 3600*24, "/", "", false, true)
	// refresh_token: 7 days, path=/refresh only, HttpOnly
	c.SetCookie("refresh_token", refreshToken, 3600*24*7, "/", "", false, true)
}

// ClearTokenCookies removes access_token and refresh_token cookies
func ClearTokenCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("access_token", "", -1, "/", "", false, true)
	c.SetCookie("refresh_token", "", -1, "/", "", false, true)
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
