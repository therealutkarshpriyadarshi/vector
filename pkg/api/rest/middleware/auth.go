package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret     string
	Enabled       bool
	PublicPaths   []string
	AdminPaths    []string
	RequireAdmin  bool
}

// Claims represents JWT claims
type Claims struct {
	UserID    string   `json:"user_id"`
	Username  string   `json:"username"`
	Roles     []string `json:"roles"`
	Namespace string   `json:"namespace,omitempty"`
	jwt.RegisteredClaims
}

// contextKey is a custom type for context keys
type contextKey string

const (
	// UserContextKey is the key for user claims in context
	UserContextKey contextKey = "user"
)

// AuthMiddleware creates a JWT authentication middleware
func AuthMiddleware(config AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication if disabled
			if !config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Check if path is public
			for _, path := range config.PublicPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSONError(w, "Missing authorization header", http.StatusUnauthorized)
				return
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				writeJSONError(w, "Invalid authorization header format", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			// Parse and validate JWT token
			token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				// Validate signing method
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(config.JWTSecret), nil
			})

			if err != nil {
				writeJSONError(w, fmt.Sprintf("Invalid token: %v", err), http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(*Claims)
			if !ok || !token.Valid {
				writeJSONError(w, "Invalid token claims", http.StatusUnauthorized)
				return
			}

			// Check if admin role is required for certain paths
			isAdminPath := false
			for _, path := range config.AdminPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					isAdminPath = true
					break
				}
			}

			if isAdminPath && !hasRole(claims.Roles, "admin") {
				writeJSONError(w, "Admin privileges required", http.StatusForbidden)
				return
			}

			// Add claims to request context
			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClaimsFromContext retrieves user claims from request context
func GetClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(UserContextKey).(*Claims)
	return claims, ok
}

// hasRole checks if user has a specific role
func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// GenerateToken creates a JWT token for testing/development
func GenerateToken(userID, username string, roles []string, namespace string, secret string) (string, error) {
	claims := &Claims{
		UserID:    userID,
		Username:  username,
		Roles:     roles,
		Namespace: namespace,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "vector-db",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"error": "%s", "status": %d}`, message, statusCode)
}
