package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
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
// 员可设置），校验通过后种一个签名 HttpOnly 解锁 Cookie：
//   - 解锁有效期 accessUnlockTTL = 0（默认）：会话 Cookie（无 Max-Age），关闭
//     浏览器即失效；前端还会在「服务端已解锁但本会话无解锁标记」时自动上锁，
//     规避浏览器「启动时恢复会话」还原会话 Cookie 造成的解锁残留；
//   - accessUnlockTTL > 0：持久 Cookie（Max-Age = TTL 秒），到期自动失效；
//   - Cookie 值 = HMAC(secret, "flatnas-access-unlock:" + 访问码 + ":" + TTL)，
//     访问码或 TTL 被修改后旧 Cookie 立即失效，无需额外黑名单。
// 访问码本身绝不回传给前端；对外只暴露 hasAccessCode / accessUnlockTTL /
// accessUnlocked（当前请求是否已解锁）。

const unlockCookieName = "flatnas_unlock"

// accessProtectionActive 返回是否已设置全局访问码（未设置 = 保护未启用，不隐藏任何分组）。
func accessProtectionActive() bool {
	return strings.TrimSpace(getCachedSystemConfig().AccessCode) != ""
}

// expectedUnlockCookieValue 计算当前访问码 + 解锁有效期对应的解锁 Cookie 期望值。
// 把 TTL 纳入 HMAC 签名：管理员调整「解锁有效期」后，所有旧解锁 Cookie 立即失效。
func expectedUnlockCookieValue() string {
	code := strings.TrimSpace(getCachedSystemConfig().AccessCode)
	ttl := normalizeUnlockTTL(getCachedSystemConfig().AccessUnlockTTL)
	mac := hmac.New(sha256.New, []byte(config.GetSecretKeyString()))
	mac.Write([]byte("flatnas-access-unlock:" + code + ":" + strconv.Itoa(ttl)))
	return hex.EncodeToString(mac.Sum(nil))
}

// normalizeUnlockTTL 约束解锁有效期取值范围：0（会话内）或 1~8760 小时（一年）。
func normalizeUnlockTTL(ttl int) int {
	if ttl < 0 {
		return 0
	}
	if ttl > 8760 {
		return 8760
	}
	return ttl
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

// sanitizedSystemConfig 返回对外可见的系统配置视图：只暴露 authMode、「是否已设
// 置访问码」、解锁有效期与「当前请求是否已解锁」，绝不回传访问码本身。
// accessUnlocked 依据请求 Cookie 实时判定，是前端区分「真解锁」与「浏览器会话
// 恢复导致的残留解锁」的关键信号。
func sanitizedSystemConfig(c *gin.Context, cfg models.SystemConfig) gin.H {
	return gin.H{
		"authMode":        cfg.AuthMode,
		"hasAccessCode":   strings.TrimSpace(cfg.AccessCode) != "",
		"accessUnlockTTL": normalizeUnlockTTL(cfg.AccessUnlockTTL),
		"accessUnlocked":  accessProtectionActive() && requestUnlocked(c),
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

// requestIsSecure 判断当前请求是否经 TLS（或经可信反代声明 X-Forwarded-Proto: https），
// 用于决定解锁 Cookie 是否加 Secure 标记：内网 HTTP 场景下若强加 Secure，浏览器将
// 拒发该 Cookie 导致解锁完全失效，故按实际协议自适应。
func requestIsSecure(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https") {
		return true
	}
	return false
}

// clientRateKey 返回用于限流的客户端标识：直接取 TCP 连接的对端地址（RemoteAddr）。
// 相对 c.ClientIP()，它不依赖可被伪造的 X-Forwarded-For / X-Real-IP，攻击者无法
// 通过轮换伪造头绕过访问码暴力枚举限流。
func clientRateKey(c *gin.Context) string {
	addr := strings.TrimSpace(c.Request.RemoteAddr)
	if addr == "" {
		return "unknown"
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
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

	ip := clientRateKey(c)
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
	// 解锁有效期：0 = 会话 Cookie（不设 Max-Age，浏览器关闭即失效）；
	// >0 = 持久 Cookie（Max-Age = TTL 秒），到期由浏览器自动清除。
	ttl := normalizeUnlockTTL(getCachedSystemConfig().AccessUnlockTTL)
	maxAge := 0
	if ttl > 0 {
		maxAge = ttl * 3600
	}
	c.SetCookie(unlockCookieName, expectedUnlockCookieValue(), maxAge, "/", "", requestIsSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"success": true, "unlocked": true})
}

// LockAccess POST /api/access/lock  清除解锁 Cookie，重新隐藏受保护分组。
func LockAccess(c *gin.Context) {
	c.SetCookie(unlockCookieName, "", -1, "/", "", requestIsSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"success": true, "unlocked": false})
}
