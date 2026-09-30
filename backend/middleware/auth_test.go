package middleware

import (
	"encoding/json"
	"os"
	"path/filepath"
	"net/http/httptest"
	"strings"
	"testing"

	"flatnasgo-backend/config"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestParseTokenRejectsUnexpectedAlg(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.SecretKey = []byte("test-secret")

	token := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"username": "admin",
	})
	signed, err := token.SignedString([]byte(config.GetSecretKeyString()))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	parsed, err := parseToken(c)
	if err != nil {
		return
	}
	if parsed != nil && parsed.Valid {
		t.Fatalf("expected invalid token, got valid token")
	}
}

func TestAuthMiddlewareRejectsDeletedUserAndStaleToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.SecretKey = []byte("test-secret")
	root := t.TempDir()
	config.DataDir = root
	config.UsersDir = filepath.Join(root, "users")
	config.SystemConfigFile = filepath.Join(root, "system.json")
	if err := os.MkdirAll(config.UsersDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.SystemConfigFile, []byte("{\"authMode\":\"multi\"}"), 0600); err != nil {
		t.Fatal(err)
	}
	userPath := filepath.Join(config.UsersDir, "alice.json")
	writeUser := func(version int64) {
		raw, err := json.Marshal(map[string]int64{"authVersion": version})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(userPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	mintToken := func(version int64) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"username":    "alice",
			"authVersion": version,
		})
		signed, err := token.SignedString([]byte(config.GetSecretKeyString()))
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	serve := func(token string) *httptest.ResponseRecorder {
		router := gin.New()
		router.GET("/", AuthMiddleware(), func(c *gin.Context) { c.Status(204) })
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	writeUser(2)
	if response := serve(mintToken(1)); response.Code != 401 {
		t.Fatalf("stale token status = %d, want 401", response.Code)
	}
	if response := serve(mintToken(2)); response.Code != 204 {
		t.Fatalf("current token status = %d, want 204", response.Code)
	}
	if err := os.Remove(userPath); err != nil {
		t.Fatal(err)
	}
	if response := serve(mintToken(2)); response.Code != 401 {
		t.Fatalf("deleted user token status = %d, want 401", response.Code)
	}
}

func TestAuthMiddlewareRejectsLegacyTokenWithoutAuthVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.SecretKey = []byte("test-secret")
	root := t.TempDir()
	config.DataDir = root
	config.UsersDir = filepath.Join(root, "users")
	config.SystemConfigFile = filepath.Join(root, "system.json")
	if err := os.MkdirAll(config.UsersDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.SystemConfigFile, []byte("{\"authMode\":\"multi\"}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.UsersDir, "alice.json"), []byte("{\"authVersion\":1}"), 0600); err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"username": "alice"})
	signed, err := token.SignedString([]byte(config.GetSecretKeyString()))
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/", AuthMiddleware(), func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 401 || !strings.Contains(response.Body.String(), "Unauthorized") {
		t.Fatalf("legacy token response = %d %q", response.Code, response.Body.String())
	}
}

func TestParseTokenAcceptsHS256(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.SecretKey = []byte("test-secret")

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "admin",
	})
	signed, err := token.SignedString([]byte(config.GetSecretKeyString()))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	parsed, err := parseToken(c)
	if err != nil || parsed == nil || !parsed.Valid {
		t.Fatalf("expected valid token, got err=%v", err)
	}
}
