package handlers

import (
	"encoding/json"
	"flatnasgo-backend/config"
	"flatnasgo-backend/utils"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 配置版本按「用户隔离」存储：文件名 = <sanitizeFileName(username)>_<id>.json。
// 这样既能被 cleanUserData 按前缀连带清理，也从根本上杜绝跨用户 IDOR
// （他人无法列出/还原/删除不属于自己的版本）。旧版遗留的纯数字文件名
// （<id>.json）仅对管理员兼容可见。

var versionIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

// versionFilePrefix 返回某用户的版本文件前缀（含结尾下划线）。
func versionFilePrefix(username string) string {
	return sanitizeFileName(username) + "_"
}

// resolveVersionFile 将 (username, id) 解析为版本文件路径；id 非法或路径逃逸时返回 false。
func resolveVersionFile(username, id string) (string, bool) {
	if !versionIDPattern.MatchString(id) {
		return "", false
	}
	name := versionFilePrefix(username) + id + ".json"
	p := filepath.Join(config.ConfigVersionsDir, name)
	root := filepath.Clean(config.ConfigVersionsDir) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(p), root) {
		return "", false
	}
	return p, true
}

type ConfigVersion struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt int64  `json:"createdAt"`
	Size      int64  `json:"size"`
}

type VersionFile struct {
	ID        string                 `json:"id"`
	Label     string                 `json:"label"`
	CreatedAt int64                  `json:"createdAt"`
	Data      map[string]interface{} `json:"data"`
}

func GetConfigVersions(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	prefix := versionFilePrefix(username)

	files, err := os.ReadDir(config.ConfigVersionsDir)
	if err != nil {
		// If dir doesn't exist, return empty list
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"versions": []ConfigVersion{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read versions directory"})
		return
	}

	var versions []ConfigVersion = []ConfigVersion{}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		name := f.Name()
		// 只返回当前用户自己的版本；管理员额外兼容旧的纯数字文件。
		isOwn := strings.HasPrefix(name, prefix)
		isLegacyAdmin := username == "admin" && versionIDPattern.MatchString(strings.TrimSuffix(name, ".json"))
		if !isOwn && !isLegacyAdmin {
			continue
		}

		// Read file to get label and created time
		content, err := os.ReadFile(filepath.Join(config.ConfigVersionsDir, name))
		if err != nil {
			continue
		}

		var vf VersionFile
		if err := json.Unmarshal(content, &vf); err != nil {
			continue
		}

		versions = append(versions, ConfigVersion{
			ID:        vf.ID,
			Label:     vf.Label,
			CreatedAt: vf.CreatedAt,
			Size:      int64(len(content)),
		})
	}

	// Sort by CreatedAt desc
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].CreatedAt > versions[j].CreatedAt
	})

	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

func SaveConfigVersion(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var payload struct {
		Label string `json:"label"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	sysConfig := getCachedSystemConfig()

	userFile := filepath.Join(config.UsersDir, username+".json")
	if username == "admin" && sysConfig.AuthMode == "single" {
		userFile = filepath.Join(config.DataDir, "data.json")
	}

	var currentData map[string]interface{}
	if err := utils.ReadJSON(userFile, &currentData); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User data not found"})
		return
	}

	now := time.Now().UnixMilli()
	id := strconv.FormatInt(now, 10)

	vf := VersionFile{
		ID:        id,
		Label:     payload.Label,
		CreatedAt: now,
		Data:      currentData,
	}

	filename := filepath.Join(config.ConfigVersionsDir, versionFilePrefix(username)+id+".json")
	if err := utils.WriteJSON(filename, vf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save version"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func RestoreConfigVersion(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var payload struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// 严格校验版本 ID（纯数字）并仅允许还原「属于当前用户」的版本文件，
	// 杜绝路径穿越与跨用户 IDOR。
	filename, ok := resolveVersionFile(username, payload.ID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version ID"})
		return
	}
	if _, err := os.Stat(filename); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}
	var vf VersionFile
	if err := utils.ReadJSON(filename, &vf); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}
	if vf.Data == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Version data corrupted"})
		return
	}

	sysConfig := getCachedSystemConfig()

	userFile := filepath.Join(config.UsersDir, username+".json")
	if username == "admin" && sysConfig.AuthMode == "single" {
		userFile = filepath.Join(config.DataDir, "data.json")
	}

	var currentData map[string]interface{}
	utils.ReadJSON(userFile, &currentData)

	newData := vf.Data

	// Preserve critical fields
	if currentData != nil {
		if pwd, ok := currentData["password"]; ok {
			newData["password"] = pwd
		}
		if usr, ok := currentData["username"]; ok {
			newData["username"] = usr
		}
	} else {
		newData["username"] = username
	}
	currentVersion := normalizeVersion(nil)
	if currentData != nil {
		currentVersion = normalizeVersion(currentData["version"])
	}
	newData["version"] = currentVersion + 1

	if err := utils.WriteJSON(userFile, newData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore version"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func DeleteConfigVersion(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID is required"})
		return
	}

	filename, ok := resolveVersionFile(username, id)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	if err := os.Remove(filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete version"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
