package services

import (
	"errors"
	"feed-system/cache"
	"feed-system/models"

	"gorm.io/gorm"
)

type FollowService struct {
	userService *UserService
}

func NewFollowService() *FollowService {
	return &FollowService{
		userService: &UserService{},
	}
}

// Follow 关注用户
func (s *FollowService) Follow(userID, followedID uint) error {
	if userID == followedID {
		return errors.New("不能关注自己")
	}

	// 检查被关注用户是否存在
	var targetUser models.User
	if err := models.DB.Where("id = ?", followedID).First(&targetUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("目标用户不存在")
		}
		return err
	}

	// 检查关注关系（包含软删除）
	var existing models.Follow
	err := models.DB.Unscoped().
		Where("user_id = ? AND followed_id = ?", userID, followedID).
		First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && !existing.DeletedAt.Valid {
		return errors.New("已经关注了该用户")
	}

	tx := models.DB.Begin()

	if err == nil && existing.DeletedAt.Valid {
		// 恢复已软删除的关注关系
		if err := tx.Unscoped().Model(&models.Follow{}).
			Where("id = ?", existing.ID).
			Update("deleted_at", nil).Error; err != nil {
			tx.Rollback()
			return errors.New("关注失败")
		}
	} else {
		// 创建新的关注关系
		follow := &models.Follow{
			UserID:     userID,
			FollowedID: followedID,
		}
		if err := tx.Create(follow).Error; err != nil {
			tx.Rollback()
			return errors.New("关注失败")
		}
	}

	// 更新关注者的关注数
	if err := tx.Model(&models.User{}).Where("id = ?", userID).
		UpdateColumn("follow_count", gorm.Expr("follow_count + 1")).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 更新被关注者的粉丝数
	if err := tx.Model(&models.User{}).Where("id = ?", followedID).
		UpdateColumn("follower_count", gorm.Expr("follower_count + 1")).Error; err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	// 更新Redis缓存
	cache.AddFollowing(userID, followedID)
	cache.AddFollower(followedID, userID)
	cache.DeleteUserCache(userID)
	cache.DeleteUserCache(followedID)

	// 更新大V状态
	s.userService.UpdateBigVStatus(followedID)

	// 关注后，需要将该用户最近的feed推送到收件箱（如果是普通用户）
	go s.backfillInbox(userID, followedID)

	return nil
}

// Unfollow 取消关注
func (s *FollowService) Unfollow(userID, followedID uint) error {
	if userID == followedID {
		return errors.New("不能取消关注自己")
	}

	// 检查是否已关注
	var follow models.Follow
	if err := models.DB.Where("user_id = ? AND followed_id = ?", userID, followedID).
		First(&follow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("尚未关注该用户")
		}
		return err
	}

	tx := models.DB.Begin()

	// 删除关注关系
	if err := tx.Delete(&follow).Error; err != nil {
		tx.Rollback()
		return errors.New("取消关注失败")
	}

	// 更新关注者的关注数
	if err := tx.Model(&models.User{}).Where("id = ?", userID).
		UpdateColumn("follow_count", gorm.Expr("GREATEST(follow_count - 1, 0)")).Error; err != nil {
		tx.Rollback()
		return err
	}

	// 更新被关注者的粉丝数
	if err := tx.Model(&models.User{}).Where("id = ?", followedID).
		UpdateColumn("follower_count", gorm.Expr("GREATEST(follower_count - 1, 0)")).Error; err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	// 更新Redis缓存
	cache.RemoveFollowing(userID, followedID)
	cache.RemoveFollower(followedID, userID)
	cache.DeleteUserCache(userID)
	cache.DeleteUserCache(followedID)

	// 更新大V状态
	s.userService.UpdateBigVStatus(followedID)

	// 取消关注后，清除收件箱中该用户的feed
	go s.cleanInbox(userID, followedID)

	return nil
}

// GetFollowers 获取粉丝列表
func (s *FollowService) GetFollowers(userID uint, page, pageSize int) ([]models.UserResponse, int64, error) {
	var follows []models.Follow
	var total int64

	query := models.DB.Model(&models.Follow{}).Where("followed_id = ?", userID)
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&follows).Error; err != nil {
		return nil, 0, err
	}

	var userIDs []uint
	for _, f := range follows {
		userIDs = append(userIDs, f.UserID)
	}

	if len(userIDs) == 0 {
		return []models.UserResponse{}, total, nil
	}

	var users []models.User
	models.DB.Where("id IN ?", userIDs).Find(&users)

	var responses []models.UserResponse
	for _, user := range users {
		responses = append(responses, user.ToResponse())
	}

	return responses, total, nil
}

// GetFollowing 获取关注列表
func (s *FollowService) GetFollowing(userID uint, page, pageSize int) ([]models.UserResponse, int64, error) {
	var follows []models.Follow
	var total int64

	query := models.DB.Model(&models.Follow{}).Where("user_id = ?", userID)
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&follows).Error; err != nil {
		return nil, 0, err
	}

	var userIDs []uint
	for _, f := range follows {
		userIDs = append(userIDs, f.FollowedID)
	}

	if len(userIDs) == 0 {
		return []models.UserResponse{}, total, nil
	}

	var users []models.User
	models.DB.Where("id IN ?", userIDs).Find(&users)

	var responses []models.UserResponse
	for _, user := range users {
		responses = append(responses, user.ToResponse())
	}

	return responses, total, nil
}

// backfillInbox 关注后回填收件箱（将被关注用户的最近feed推入当前用户收件箱）
func (s *FollowService) backfillInbox(userID, followedID uint) {
	// 检查被关注者是否为大V
	isBigV, _ := cache.IsBigV(followedID)
	if isBigV {
		// 大V不需要回填，拉取时合并
		return
	}

	// 查询被关注者最近的feed
	var feeds []models.Feed
	models.DB.Where("user_id = ?", followedID).
		Order("created_at DESC").
		Limit(50).
		Find(&feeds)

	for _, feed := range feeds {
		timestamp := float64(feed.CreatedAt.UnixMilli())
		cache.AddToInbox(userID, feed.ID, timestamp)
	}
}

// cleanInbox 取消关注后清理收件箱
func (s *FollowService) cleanInbox(userID, unfollowedID uint) {
	// 查询被取关者的feed
	var feeds []models.Feed
	models.DB.Where("user_id = ?", unfollowedID).
		Select("id").
		Find(&feeds)

	for _, feed := range feeds {
		cache.RemoveFromInbox(userID, feed.ID)
	}

	// 清理Timeline表
	models.DB.Where("user_id = ? AND author_id = ?", userID, unfollowedID).
		Delete(&models.Timeline{})
}
