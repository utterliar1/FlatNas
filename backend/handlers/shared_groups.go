package handlers

import (
	"encoding/json"
	"flatnasgo-backend/config"
	"flatnasgo-backend/utils"
	"os"
	"path/filepath"
	"time"
)

// 共享分组（多用户共同的书签分组）
//
// 设计：共享分组并不是独立的一份存储，而是「管理员自己分组上的一个 shared 标记」。
//   - 管理员在分组设置里把某个分组标记为 shared，即表示该组整组共享给所有用户；
//   - 该标记随管理员的 groups 一起保存在管理员自己的数据文件里，管理员仍用现有 UI 正常编辑；
//   - 其他登录用户 / 访客通过 GetData 注入的 sharedGroups 字段拿到只读副本，不写入自己的数据文件。
// 这样既复用了现有分组编辑链路，又天然满足「仅管理员可维护」的权限约束。

// adminDataFile 返回管理员的数据文件路径。
// 单用户模式下管理员数据在 data.json；多用户模式下在 users/admin.json。
func adminDataFile() string {
	sysConfig := getCachedSystemConfig()
	if sysConfig.AuthMode == "single" {
		return filepath.Join(config.DataDir, "data.json")
	}
	return filepath.Join(config.UsersDir, "admin.json")
}

// sharedGroupsModTime 返回管理员数据文件的修改时间。
// 用于让非管理员用户的 GetData 缓存与 ETag 能感知共享分组的变更。
func sharedGroupsModTime() time.Time {
	info, err := os.Stat(adminDataFile())
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// extractSharedGroups 从一份用户数据中提取所有被标记为 shared 的分组（整组共享）。
func extractSharedGroups(data map[string]interface{}) []interface{} {
	if data == nil {
		return nil
	}
	groups, ok := data["groups"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]interface{}, 0)
	for _, g := range groups {
		gm, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		if shared, ok := gm["shared"].(bool); ok && shared {
			result = append(result, gm)
		}
	}
	return result
}

// loadSharedGroups 读取管理员数据文件并返回其共享分组。
func loadSharedGroups() []interface{} {
	var adminData map[string]interface{}
	if err := utils.ReadJSON(adminDataFile(), &adminData); err != nil {
		return nil
	}
	return extractSharedGroups(adminData)
}

// sharedGroupsSignature 用于判断两次保存之间共享分组是否发生变化（是否需广播给其他用户）。
func sharedGroupsSignature(data map[string]interface{}) string {
	list := extractSharedGroups(data)
	if list == nil {
		list = []interface{}{}
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return ""
	}
	return string(raw)
}

// sharedGroupsInjectedFor 判断当前请求是否应从管理员数据注入共享分组。
// 管理员本人的请求不需要注入（共享分组本就在他自己的 groups 里）；
// 访客（用户名被置为 admin）读取的就是管理员文件，也不需要注入。
func sharedGroupsInjectedFor(username string, isGuest bool) bool {
	if isGuest {
		return false
	}
	return username != "" && username != "admin"
}
