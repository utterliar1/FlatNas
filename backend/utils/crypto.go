package utils

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword 生成 bcrypt 密码散列（cost 14）。
// 校验侧不在这里：handlers/auth.go 直接调用 bcrypt.CompareHashAndPassword。
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}
