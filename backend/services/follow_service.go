package services

import (
	"errors"
	"feed/cache"
	"feed/models"
	"feed/repository"

	"gorm.io/gorm"
)

// FollowService 负责关注关系域业务编排。
// 包含：关注、取关、列表查询，以及收件箱回填/清理等副作用流程。
type FollowService struct {
	followRepo          repository.FollowRepository
	userService         *UserService
	notificationService *NotificationService
}

func NewFollowService() *FollowService {
	return &FollowService{
		followRepo:          repository.NewFollowRepository(models.DB),
		userService:         NewUserService(),
		notificationService: NewNotificationService(),
	}
}

// Follow 关注用户
func (s *FollowService) Follow(userID, followedID uint) error {
	if userID == followedID {
		return errors.New("不能关注自己")
	}

	if _, err := s.followRepo.GetTargetUser(followedID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("目标用户不存在")
		}
		return err
	}

	existing, err := s.followRepo.GetAnyRelation(userID, followedID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && !existing.DeletedAt.Valid {
		return errors.New("已经关注了该用户")
	}

	tx := s.followRepo.BeginTx()

	if err == nil && existing.DeletedAt.Valid {
		if err := s.followRepo.RestoreRelation(tx, existing.ID); err != nil {
			tx.Rollback()
			return errors.New("关注失败")
		}
	} else {
		follow := &models.Follow{UserID: userID, FollowedID: followedID}
		if err := s.followRepo.CreateRelation(tx, follow); err != nil {
			tx.Rollback()
			return errors.New("关注失败")
		}
	}

	if err := s.followRepo.IncreaseFollowCount(tx, userID); err != nil {
		tx.Rollback()
		return err
	}
	if err := s.followRepo.IncreaseFollowerCount(tx, followedID); err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	cache.AddFollowing(userID, followedID)
	cache.AddFollower(followedID, userID)
	cache.DeleteUserCache(userID)
	cache.DeleteUserCache(followedID)
	go s.refreshFollowRelationCache(userID)
	go s.refreshFollowerRelationCache(followedID)

	s.userService.UpdateBigVStatus(followedID)
	s.notificationService.CreateFollowNotification(userID, followedID)
	go s.backfillInbox(userID, followedID)

	return nil
}

// Unfollow 取消关注
func (s *FollowService) Unfollow(userID, followedID uint) error {
	if userID == followedID {
		return errors.New("不能取消关注自己")
	}

	relation, err := s.followRepo.GetAnyRelation(userID, followedID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("尚未关注该用户")
		}
		return err
	}
	if relation.DeletedAt.Valid {
		return errors.New("尚未关注该用户")
	}

	tx := s.followRepo.BeginTx()
	if err := s.followRepo.DeleteRelation(tx, relation); err != nil {
		tx.Rollback()
		return errors.New("取消关注失败")
	}
	if err := s.followRepo.DecreaseFollowCount(tx, userID); err != nil {
		tx.Rollback()
		return err
	}
	if err := s.followRepo.DecreaseFollowerCount(tx, followedID); err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()

	cache.RemoveFollowing(userID, followedID)
	cache.RemoveFollower(followedID, userID)
	cache.DeleteUserCache(userID)
	cache.DeleteUserCache(followedID)
	go s.refreshFollowRelationCache(userID)
	go s.refreshFollowerRelationCache(followedID)

	s.userService.UpdateBigVStatus(followedID)
	go s.cleanInbox(userID, followedID)

	return nil
}

// GetFollowers 获取粉丝列表
func (s *FollowService) GetFollowers(userID uint, page, pageSize int) ([]models.UserResponse, int64, error) {
	follows, total, err := s.followRepo.ListFollowers(userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	userIDs := make([]uint, 0, len(follows))
	for _, f := range follows {
		userIDs = append(userIDs, f.UserID)
	}

	users, err := s.followRepo.ListUsersByIDs(userIDs)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]models.UserResponse, 0, len(users))
	for _, user := range users {
		responses = append(responses, user.ToResponse())
	}

	return responses, total, nil
}

// GetFollowing 获取关注列表
func (s *FollowService) GetFollowing(userID uint, page, pageSize int) ([]models.UserResponse, int64, error) {
	follows, total, err := s.followRepo.ListFollowing(userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	userIDs := make([]uint, 0, len(follows))
	for _, f := range follows {
		userIDs = append(userIDs, f.FollowedID)
	}

	users, err := s.followRepo.ListUsersByIDs(userIDs)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]models.UserResponse, 0, len(users))
	for _, user := range users {
		responses = append(responses, user.ToResponse())
	}

	return responses, total, nil
}

func (s *FollowService) backfillInbox(userID, followedID uint) {
	isBigV, _ := cache.IsBigV(followedID)
	if isBigV {
		return
	}

	feeds, err := s.followRepo.ListRecentFeedsByUser(followedID, 50)
	if err != nil {
		return
	}
	for _, feed := range feeds {
		_ = cache.AddToInbox(userID, feed.ID, float64(feed.CreatedAt.UnixMilli()))
	}
}

func (s *FollowService) cleanInbox(userID, unfollowedID uint) {
	feeds, err := s.followRepo.ListFeedIDsByUser(unfollowedID)
	if err != nil {
		return
	}
	for _, feed := range feeds {
		cache.RemoveFromInbox(userID, feed.ID)
	}
	_ = s.followRepo.DeleteTimelineByUserAndAuthor(userID, unfollowedID)
}

// refreshFollowRelationCache 回源数据库并批量刷新“我关注的人”缓存。
func (s *FollowService) refreshFollowRelationCache(userID uint) {
	follows, _, err := s.followRepo.ListFollowing(userID, 1, 5000)
	if err != nil {
		return
	}
	ids := make([]any, 0, len(follows))
	for _, f := range follows {
		ids = append(ids, f.FollowedID)
	}
	_ = cache.SetFollowing(userID, ids)
}

// refreshFollowerRelationCache 回源数据库并批量刷新“谁关注我”缓存。
func (s *FollowService) refreshFollowerRelationCache(userID uint) {
	follows, _, err := s.followRepo.ListFollowers(userID, 1, 5000)
	if err != nil {
		return
	}
	ids := make([]any, 0, len(follows))
	for _, f := range follows {
		ids = append(ids, f.UserID)
	}
	_ = cache.SetFollowers(userID, ids)
}
