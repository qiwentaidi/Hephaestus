package admin

import (
	"hephaestus-backend/handlers/platform"
	"hephaestus-backend/middleware"
	systemauth "hephaestus-backend/pkg/auth"
	"hephaestus-backend/pkg/database"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// validatePassword 验证密码复杂性
// 要求：至少8位，包含大写字母、小写字母、数字
func validatePassword(password string) (bool, string) {
	if len(password) < 8 {
		return false, "密码长度至少为8位"
	}

	var hasUpper, hasLower, hasNumber bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		}
	}

	if !hasUpper {
		return false, "密码必须包含至少一个大写字母"
	}
	if !hasLower {
		return false, "密码必须包含至少一个小写字母"
	}
	if !hasNumber {
		return false, "密码必须包含至少一个数字"
	}

	return true, ""
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=8"`
}

// UpdateUserRequest 更新用户信息请求
type UpdateUserRequest struct {
	Email string `json:"email"`
}

// LoginHandler 用户登录
func LoginHandler(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "数据库连接失败",
		})
		return
	}

	// 查询用户
	var user database.User
	if err := db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "用户名或密码错误",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询用户失败",
		})
		return
	}

	// 验证密码
	if user.Password != systemauth.HashPassword(req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "用户名或密码错误",
		})
		return
	}

	// 生成 JWT token
	token, err := middleware.GenerateToken(user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "生成token失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
			"token":    token,
		},
	})
}

// GetCurrentUserHandler 获取当前用户信息
func GetCurrentUserHandler(c *gin.Context) {
	username := platform.Username(c)
	if username == "" {
		platform.WriteUnauthorized(c)
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "数据库连接失败",
		})
		return
	}

	var user database.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "用户不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询用户失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		},
	})
}

// ChangePasswordHandler 修改密码
func ChangePasswordHandler(c *gin.Context) {
	username := platform.Username(c)
	if username == "" {
		platform.WriteUnauthorized(c)
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误，密码长度至少8位",
		})
		return
	}

	// 验证新密码复杂性
	if valid, msg := validatePassword(req.NewPassword); !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   msg,
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "数据库连接失败",
		})
		return
	}

	// 查询用户
	var user database.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "用户不存在",
		})
		return
	}

	// 验证旧密码
	if user.Password != systemauth.HashPassword(req.OldPassword) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "原密码错误",
		})
		return
	}

	// 更新密码
	if err := db.Model(&user).Update("password", systemauth.HashPassword(req.NewPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "修改密码失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "密码修改成功",
	})
}

// UpdateUserHandler 更新用户信息
func UpdateUserHandler(c *gin.Context) {
	username := platform.Username(c)
	if username == "" {
		platform.WriteUnauthorized(c)
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "数据库连接失败",
		})
		return
	}

	// 更新用户信息
	result := db.Model(&database.User{}).Where("username = ?", username).Updates(map[string]interface{}{
		"email": req.Email,
	})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新用户信息失败",
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "用户不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "用户信息更新成功",
	})
}

// InitAdminHandler 初始化管理员账户
func InitAdminHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误，用户名和密码必填，密码长度至少8位",
		})
		return
	}

	// 验证密码复杂性
	if valid, msg := validatePassword(req.Password); !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   msg,
		})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "数据库连接失败",
		})
		return
	}

	// 检查是否已有用户
	var count int64
	db.Model(&database.User{}).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "系统已初始化，请直接登录",
		})
		return
	}

	// 创建管理员用户
	user := database.User{
		Username: strings.TrimSpace(req.Username),
		Password: systemauth.HashPassword(req.Password),
		Role:     "admin",
	}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建管理员账户失败",
		})
		return
	}

	// 生成 JWT token
	token, err := middleware.GenerateToken(user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "生成token失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
			"token":    token,
		},
	})
}

// CheckInitHandler 检查系统是否已初始化
func CheckInitHandler(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "数据库连接失败",
		})
		return
	}

	var count int64
	db.Model(&database.User{}).Count(&count)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"initialized": count > 0,
	})
}
