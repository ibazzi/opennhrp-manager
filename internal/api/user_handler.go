package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"opennhrp-manager/internal/auth"
	"opennhrp-manager/internal/db"
)

type UserHandler struct {
	db *db.DB
}

func NewUserHandler(database *db.DB) *UserHandler {
	return &UserHandler{
		db: database,
	}
}

type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"` // admin, readonly
}

type UpdateUserRequest struct {
	Role     string `json:"role"`
	Password string `json:"password"`
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.db.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户列表失败"})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写完整的用户信息"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
		return
	}

	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码长度至少需要 6 位"})
		return
	}

	if req.Role != "admin" && req.Role != "readonly" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "角色必须为 'admin' (管理员) 或 'readonly' (只读用户)"})
		return
	}

	// Check if username exists
	existing, _ := h.db.GetUserByUsername(req.Username)
	if existing != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该用户名已存在"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	user := &db.UserRecord{
		ID:           "u-" + uuid.New().String()[:8],
		Username:     req.Username,
		PasswordHash: hash,
		Role:         req.Role,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.db.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建用户失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户 ID 不能为空"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}

	targetUser, err := h.db.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标用户不存在"})
		return
	}

	role := targetUser.Role
	if req.Role != "" {
		if req.Role != "admin" && req.Role != "readonly" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "角色必须为 'admin' (管理员) 或 'readonly' (只读用户)"})
			return
		}
		// If changing from admin to readonly, verify there's at least one other admin
		if targetUser.Role == "admin" && req.Role == "readonly" {
			adminCount, _ := h.db.CountAdmins()
			if adminCount <= 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "无法降级：系统中必须保留至少一名管理员"})
				return
			}
		}
		role = req.Role
	}

	var passwordHash string
	if req.Password != "" {
		if len(req.Password) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "新密码长度至少需要 6 位"})
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
			return
		}
		passwordHash = hash
	}

	if err := h.db.UpdateUser(id, role, passwordHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新用户失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "用户信息更新成功"})
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户 ID 不能为空"})
		return
	}

	currentUserID, _ := c.Get("user_id")
	if currentUserID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "禁止删除当前登录的自身账号"})
		return
	}

	targetUser, err := h.db.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标用户不存在"})
		return
	}

	if targetUser.Role == "admin" {
		adminCount, _ := h.db.CountAdmins()
		if adminCount <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无法删除：系统中必须保留至少一名管理员"})
			return
		}
	}

	if err := h.db.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "用户删除成功"})
}
