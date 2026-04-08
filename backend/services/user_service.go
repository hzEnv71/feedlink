package services

import (
	"errors"
	"feed-system/cache"
	"feed-system/config"
	"feed-system/models"
	"feed-system/utils"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserService struct{}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6,max=50"`
	Nickname string `json:"nickname" binding:"required,min=1,max=100"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token string              `json:"token"`
	User  models.UserResponse `json:"user"`
}

// UpdateProfileRequest 更新个人资料请求
type UpdateProfileRequest struct {
	Avatar *string `json:"avatar" binding:"omitempty,max=500"`
	Bio    *string `json:"bio" binding:"omitempty,max=500"`
}

// Register 用户注册
func (s *UserService) Register(req *RegisterRequest) (*models.User, error) {
	// 检查用户名是否已存在
	var count int64
	models.DB.Model(&models.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		return nil, errors.New("用户名已存在")
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("密码加密失败")
	}

	user := &models.User{
		Username: req.Username,
		Password: string(hashedPassword),
		Nickname: req.Nickname,
	}

	if err := models.DB.Create(user).Error; err != nil {
		return nil, errors.New("注册失败")
	}

	return user, nil
}

// Login 用户登录
func (s *UserService) Login(req *LoginRequest) (*LoginResponse, error) {
	var user models.User
	if err := models.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, errors.New("查询用户失败")
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, errors.New("密码错误")
	}

	// 生成Token
	token, err := utils.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, errors.New("生成Token失败")
	}

	return &LoginResponse{
		Token: token,
		User:  user.ToResponse(),
	}, nil
}

// GetUserByID 根据ID获取用户
func (s *UserService) GetUserByID(userID uint) (*models.User, error) {
	var user models.User
	if err := models.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, err
	}
	return &user, nil
}

// GetUserProfile 获取用户资料（含是否关注状态）
func (s *UserService) GetUserProfile(targetUserID, currentUserID uint) (*models.UserResponse, error) {
	user, err := s.GetUserByID(targetUserID)
	if err != nil {
		return nil, err
	}

	resp := user.ToResponse()

	// 检查当前用户是否关注了目标用户
	if currentUserID > 0 && currentUserID != targetUserID {
		isFollowed, _ := cache.IsFollowing(currentUserID, targetUserID)
		if !isFollowed {
			// Redis没有数据，从数据库查询
			var count int64
			models.DB.Model(&models.Follow{}).
				Where("user_id = ? AND followed_id = ?", currentUserID, targetUserID).
				Count(&count)
			isFollowed = count > 0
		}
		resp.IsFollowed = isFollowed
	}

	return &resp, nil
}

// UpdateBigVStatus 更新用户大V状态
func (s *UserService) UpdateBigVStatus(userID uint) error {
	var user models.User
	if err := models.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return err
	}

	threshold := int64(config.AppConfig.Feed.BigVThreshold)
	isBigV := user.FollowerCount >= threshold

	if user.IsBigV != isBigV {
		models.DB.Model(&user).Update("is_big_v", isBigV)
		cache.SetBigV(userID, isBigV)
		cache.DeleteUserCache(userID)
	}

	return nil
}

type ProfileVisitResponse struct {
	ID        uint                `json:"id"`
	VisitedAt string              `json:"visited_at"`
	Visitor   models.UserResponse `json:"visitor"`
}

// SearchUsers 搜索用户
func (s *UserService) SearchUsers(keyword string, page, pageSize int) ([]models.UserResponse, int64, error) {
	var users []models.User
	var total int64

	query := models.DB.Model(&models.User{}).Where("username LIKE ? OR nickname LIKE ?",
		"%"+keyword+"%", "%"+keyword+"%")

	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	var responses []models.UserResponse
	for _, user := range users {
		responses = append(responses, user.ToResponse())
	}

	return responses, total, nil
}

// UpdateProfile 更新个人资料
func (s *UserService) UpdateProfile(userID uint, req *UpdateProfileRequest) (*models.User, error) {
	user, err := s.GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}
	if req.Bio != nil {
		updates["bio"] = *req.Bio
	}

	if len(updates) == 0 {
		return user, nil
	}

	if err := models.DB.Model(user).Updates(updates).Error; err != nil {
		return nil, errors.New("更新个人资料失败")
	}

	if err := models.DB.Where("id = ?", userID).First(user).Error; err != nil {
		return nil, errors.New("获取更新后的用户信息失败")
	}

	return user, nil
}

// RecordProfileVisit 记录主页访问
func (s *UserService) RecordProfileVisit(visitorID, targetUserID uint) error {
	if visitorID == 0 || targetUserID == 0 || visitorID == targetUserID {
		return nil
	}

	visit := models.ProfileVisit{
		VisitorID:    visitorID,
		TargetUserID: targetUserID,
		VisitedAt:    time.Now(),
	}

	return models.DB.Where("visitor_id = ? AND target_user_id = ?", visitorID, targetUserID).
		Assign(models.ProfileVisit{VisitedAt: visit.VisitedAt}).
		FirstOrCreate(&visit).Error
}

// GetRecentVisitors 获取最近访客
func (s *UserService) GetRecentVisitors(targetUserID uint, page, pageSize int) ([]ProfileVisitResponse, int64, error) {
	var visits []models.ProfileVisit
	var total int64

	query := models.DB.Model(&models.ProfileVisit{}).Where("target_user_id = ?", targetUserID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Preload("Visitor").Order("visited_at DESC").Offset(offset).Limit(pageSize).Find(&visits).Error; err != nil {
		return nil, 0, err
	}

	result := make([]ProfileVisitResponse, 0, len(visits))
	for _, v := range visits {
		result = append(result, ProfileVisitResponse{
			ID:        v.ID,
			VisitedAt: v.VisitedAt.Format(time.RFC3339),
			Visitor:   v.Visitor.ToResponse(),
		})
	}

	return result, total, nil
}
