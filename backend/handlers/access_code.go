package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"flatnasgo-backend/config"
	"flatnasgo-backend/models"

	"github.com/gin-gonic/gin"
)

// 访问码保护（隐藏分组）
//
// 设计：用户可在自己的任意分组上标记 protected，被标记的分组在「未解锁」状态下
// 由后端从 /api/data 响应中整体剔除（对所有人生效，含管理员与访客），前端完全
// 拿不到数据，而非仅前端隐藏。解锁方式为提交全局访问码（存于系统配置，仅管理
// 员可设置），校验通过后种一个签名 HttpOnly 会话 Cookie：
//   - 会话 Cookie（无 Max-Age）：关闭浏览器后自动失效，符合「会话内有效」；
//   - Cookie 值 = HMAC(secret, "flatnas-access-unlock:" + 当前访问码)，
//     访问码被修改后旧 Cookie 立即失效，无需额外黑名单。
// 访问码本身绝不回传给前端；对外只暴露 hasAccessCode 布尔值。

const unlockCookieName = "flatnas_unlock"

// accessProtectionActive 返回是否已设置全局访问码（未设置 = 保护未启用，不隐藏任何分组）。
func accessProtectionActive() bool {
	return strings.TrimSpace(getCachedSystemConfig().AccessCode) != ""
}

// expectedUnlockCookieValue 计算当前访问码对应的解锁 Cookie 期望值。
func expectedUnlockCookieValue() string {
	code := strings.TrimSpace(getCachedSystemConfig().AccessCode)
	mac := hmac.New(sha256.New, []byte(config.GetSecretKeyString()))
	mac.Write([]byte("flatnas-access-unlock:" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

// requestUnlocked 判断当前请求是否处于「已解锁」状态。
// 保护未启用时返回 false，但调用方只在保护启用时才依赖该值做过滤。
func requestUnlocked(c *gin.Context) bool {
	ck, err := c.Cookie(unlockCookieName)
	if err != nil || ck == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(ck), []byte(expectedUnlockCookieValue())) == 1
}

// filterProtectedGroups 从分组列表中剔除被标记为 protected 的分组（未解锁时调用）。
func filterProtectedGroups(groups []interface{}) []interface{} {
	if groups == nil {
		return groups
	}
	result := make([]interface{}, 0, len(groups))
	for _, g := range groups {
		gm, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		if p, ok := gm["protected"].(bool); ok && p {
			continue
		}
		result = append(result, g)
	}
	return result
}

// sanitizedSystemConfig 返回对外可见的系统配置视图：只暴露 authMode 与
// 「是否已设置访问码」，绝不回传访问码本身。
func sanitizedSystemConfig(cfg models.SystemConfig) gin.H {
	return gin.H{
		"authMode":      cfg.AuthMode,
		"hasAccessCode": strings.TrimSpace(cfg.AccessCode) != "",
	}
}

// --- 解锁尝试限流（防暴力枚举，进程内实现即可） ---

var (
	unlockAttemptMu sync.Mutex
	unlockAttempts  = map[string][]time.Time{} // key: 客户端 IP
)

const (
	unlockAttemptWindow = time.Minute
	unlockAttemptLimit  = 10
)

func unlockRateLimited(ip string) bool {
	now := time.Now()
	unlockAttemptMu.Lock()
	defer unlockAttemptMu.Unlock()
	recent := unlockAttempts[ip][:0]
	for _, t := range unlockAttempts[ip] {
		if now.Sub(t) < unlockAttemptWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= unlockAttemptLimit {
		unlockAttempts[ip] = recent
		return true
	}
	// 未超限时也记录本次尝试（成功解锁时会清空）
	recent = append(recent, now)
	unlockAttempts[ip] = recent
	return false
}

func unlockClearAttempts(ip string) {
	unlockAttemptMu.Lock()
	delete(unlockAttempts, ip)
	unlockAttemptMu.Unlock()
}

// --- HTTP handlers ---

// UnlockAccess POST /api/access/unlock  body: {"code": "..."}
// 校验全局访问码，通过后种会话 Cookie。
func UnlockAccess(c *gin.Context) {
	var payload struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if !accessProtectionActive() {
		// 未设置访问码 = 无保护，直接视为已解锁（不种 Cookie，避免残留）
		c.JSON(http.StatusOK, gin.H{"success": true, "unlocked": true})
		return
	}

	ip := c.ClientIP()
	if unlockRateLimited(ip) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "尝试过于频繁，请稍后再试"})
		return
	}

	code := strings.TrimSpace(payload.Code)
	expected := strings.TrimSpace(getCachedSystemConfig().AccessCode)
	if subtle.ConstantTimeCompare([]byte(code), []byte(expected)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "访问码不正确"})
		return
	}

	unlockClearAttempts(ip)
	// 会话 Cookie：不设 Max-Age，浏览器关闭即失效
	c.SetCookie(unlockCookieName, expectedUnlockCookieValue(), 0, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"success": true, "unlocked": true})
}

// LockAccess POST /api/access/lock  清除解锁 Cookie，重新隐藏受保护分组。
func LockAccess(c *gin.Context) {
	c.SetCookie(unlockCookieName, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"success": true, "unlocked": false})
}
