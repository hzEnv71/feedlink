package services

import (
	"errors"
	"feed/cache"
	"feed/models"
	"feed/mq"
	"log"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type FeedService struct {
	userService *UserService
}

func NewFeedService() *FeedService {
	return &FeedService{
		userService: &UserService{},
	}
}

// CreateFeedRequest 发布Feed请求
type CreateFeedRequest struct {
	Content string `json:"content"`
	Images  string `json:"images"`
	Videos  string `json:"videos"`
}

// UpdateFeedRequest 编辑Feed请求
type UpdateFeedRequest struct {
	Content string `json:"content"`
	Images  string `json:"images"`
	Videos  string `json:"videos"`
}

// RepostFeedRequest 转发Feed请求
type RepostFeedRequest struct {
	Content    string `json:"content"` // 转发时的评论
	OriginalID uint   `json:"original_id" binding:"required"`
}

// PublishFeed 发布Feed
func (s *FeedService) PublishFeed(userID uint, req *CreateFeedRequest) (*models.Feed, error) {
	// 至少要有文案、图片或视频之一
	if req.Content == "" && req.Images == "" && req.Videos == "" {
		return nil, errors.New("请输入文案、上传图片或视频")
	}

	feed := &models.Feed{
		UserID:   userID,
		Content:  req.Content,
		Images:   req.Images,
		Videos:   req.Videos,
		FeedType: models.FeedTypeOriginal,
	}

	if err := models.DB.Create(feed).Error; err != nil {
		return nil, errors.New("发布失败")
	}

	// 异步推送到粉丝收件箱（通过MQ）
	mq.PublishFeed(feed.ID, userID)

	return feed, nil
}

// UpdateFeed 编辑Feed
func (s *FeedService) UpdateFeed(feedID, userID uint, req *UpdateFeedRequest) (*models.Feed, error) {
	var feed models.Feed
	if err := models.DB.Where("id = ? AND user_id = ?", feedID, userID).First(&feed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("动态不存在或无权限编辑")
		}
		return nil, err
	}

	// 转发类型的Feed不允许编辑图片/视频
	updates := map[string]interface{}{
		"content": req.Content,
	}
	if feed.FeedType == models.FeedTypeOriginal {
		updates["images"] = req.Images
		updates["videos"] = req.Videos
	}

	if err := models.DB.Model(&feed).Updates(updates).Error; err != nil {
		return nil, errors.New("编辑失败")
	}

	// 重新查询
	models.DB.Where("id = ?", feedID).First(&feed)
	return &feed, nil
}

// RepostFeed 转发Feed
func (s *FeedService) RepostFeed(userID uint, req *RepostFeedRequest) (*models.Feed, error) {
	// 检查原始Feed是否存在
	var originalFeed models.Feed
	if err := models.DB.Where("id = ?", req.OriginalID).First(&originalFeed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("原始动态不存在")
		}
		return nil, err
	}

	// 如果原始Feed本身是转发，则转发原始的那条
	originalID := req.OriginalID
	if originalFeed.FeedType == models.FeedTypeRepost && originalFeed.OriginalID != nil {
		originalID = *originalFeed.OriginalID
	}

	feed := &models.Feed{
		UserID:     userID,
		Content:    req.Content,
		FeedType:   models.FeedTypeRepost,
		OriginalID: &originalID,
	}

	tx := models.DB.Begin()

	if err := tx.Create(feed).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("转发失败")
	}

	// 增加原始Feed的转发计数
	if err := tx.Model(&models.Feed{}).Where("id = ?", originalID).
		UpdateColumn("share_count", gorm.Expr("share_count + 1")).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	tx.Commit()

	// 异步推送到粉丝收件箱
	mq.PublishFeed(feed.ID, userID)

	return feed, nil
}

// DeleteFeed 删除Feed
func (s *FeedService) DeleteFeed(feedID, userID uint) error {
	var feed models.Feed
	if err := models.DB.Where("id = ? AND user_id = ?", feedID, userID).First(&feed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("动态不存在或无权限删除")
		}
		return err
	}

	if err := models.DB.Delete(&feed).Error; err != nil {
		return errors.New("删除失败")
	}

	// 清理相关缓存和Timeline
	go func() {
		// 从作者发件箱移除
		key := strconv.FormatUint(uint64(feedID), 10)
		_ = cache.RedisClient.ZRem(cache.Ctx, "outbox:"+strconv.FormatUint(uint64(userID), 10), key).Err()

		// 从所有粉丝收件箱移除
		var timelines []models.Timeline
		models.DB.Where("feed_id = ?", feedID).Find(&timelines)
		for _, tl := range timelines {
			cache.RemoveFromInbox(tl.UserID, feedID)
		}
		models.DB.Where("feed_id = ?", feedID).Delete(&models.Timeline{})
	}()

	return nil
}

// GetFeedByID 获取单个Feed详情
func (s *FeedService) GetFeedByID(feedID, currentUserID uint) (*models.FeedResponse, error) {
	var feed models.Feed
	if err := models.DB.Where("id = ?", feedID).First(&feed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("动态不存在")
		}
		return nil, err
	}

	return s.buildFeedResponse(&feed, currentUserID), nil
}

// GetUserFeeds 获取用户发布的Feed列表（发件箱）
func (s *FeedService) GetUserFeeds(userID uint, page, pageSize int, currentUserID uint) ([]models.FeedResponse, int64, error) {
	var feeds []models.Feed
	var total int64

	query := models.DB.Model(&models.Feed{}).Where("user_id = ?", userID)
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&feeds).Error; err != nil {
		return nil, 0, err
	}

	responses := s.buildFeedResponses(feeds, currentUserID)
	return responses, total, nil
}

// GetTimeline 获取用户时间线（推拉混合）- 包含本人和关注的人的Feed
func (s *FeedService) GetTimeline(userID uint, page, pageSize int) ([]models.FeedResponse, int64, error) {
	offset := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	// ==================== 第一部分：获取本人的Feed ====================
	var ownFeeds []models.Feed
	models.DB.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(int(limit) + 20).
		Find(&ownFeeds)

	var ownFeedIDs []uint
	for _, f := range ownFeeds {
		ownFeedIDs = append(ownFeedIDs, f.ID)
	}

	// ==================== 第二部分：从收件箱获取推模式的feed ====================
	inboxFeedIDs, err := cache.GetInbox(userID, 0, limit+50)
	if err != nil {
		log.Printf("Get inbox from redis failed: %v, fallback to DB", err)
	}

	var pushFeedIDs []uint
	for _, idStr := range inboxFeedIDs {
		id, _ := strconv.ParseUint(idStr, 10, 64)
		if id > 0 {
			pushFeedIDs = append(pushFeedIDs, uint(id))
		}
	}

	// 如果Redis收件箱为空，从数据库Timeline表回源
	if len(pushFeedIDs) == 0 {
		var timelines []models.Timeline
		models.DB.Where("user_id = ?", userID).
			Order("created_at DESC").
			Limit(int(limit) + 50).
			Find(&timelines)

		for _, tl := range timelines {
			pushFeedIDs = append(pushFeedIDs, tl.FeedID)
		}
	}

	// ==================== 第三部分：拉取关注的大V的发件箱 ====================
	pullFeedIDs := s.pullBigVFeeds(userID, limit)

	// ==================== 第四部分：合并去重排序 ====================
	allFeedIDs := s.mergeAndDedup(ownFeedIDs, pushFeedIDs, pullFeedIDs)

	if len(allFeedIDs) == 0 {
		return []models.FeedResponse{}, 0, nil
	}

	// 查询Feed详情
	var feeds []models.Feed
	models.DB.Where("id IN ?", allFeedIDs).Find(&feeds)

	// 按创建时间倒序排列
	sort.Slice(feeds, func(i, j int) bool {
		return feeds[i].CreatedAt.After(feeds[j].CreatedAt)
	})

	// 总数和分页截取
	total := int64(len(feeds))
	start := int(offset)
	if start >= len(feeds) {
		return []models.FeedResponse{}, total, nil
	}
	end := start + int(limit)
	if end > len(feeds) {
		end = len(feeds)
	}
	feeds = feeds[start:end]

	responses := s.buildFeedResponses(feeds, userID)
	return responses, total, nil
}

// pullBigVFeeds 拉取关注的大V的发件箱feed
func (s *FeedService) pullBigVFeeds(userID uint, limit int64) []uint {
	var follows []models.Follow
	models.DB.Where("user_id = ?", userID).Find(&follows)

	var pullFeedIDs []uint

	for _, follow := range follows {
		isBigV, err := cache.IsBigV(follow.FollowedID)
		if err != nil {
			var user models.User
			if dbErr := models.DB.Select("is_big_v").Where("id = ?", follow.FollowedID).First(&user).Error; dbErr == nil {
				isBigV = user.IsBigV
			}
		}

		if !isBigV {
			continue
		}

		outboxIDs, err := cache.GetOutbox(follow.FollowedID, 0, limit)
		if err != nil {
			log.Printf("Get outbox of user %d failed: %v", follow.FollowedID, err)
			continue
		}

		for _, idStr := range outboxIDs {
			id, _ := strconv.ParseUint(idStr, 10, 64)
			if id > 0 {
				pullFeedIDs = append(pullFeedIDs, uint(id))
			}
		}

		if len(outboxIDs) == 0 {
			var feeds []models.Feed
			models.DB.Where("user_id = ?", follow.FollowedID).
				Order("created_at DESC").
				Limit(int(limit)).
				Select("id").
				Find(&feeds)

			for _, feed := range feeds {
				pullFeedIDs = append(pullFeedIDs, feed.ID)
			}
		}
	}

	return pullFeedIDs
}

// mergeAndDedup 合并去重（支持多个来源）
func (s *FeedService) mergeAndDedup(idGroups ...[]uint) []uint {
	seen := make(map[uint]bool)
	var result []uint

	for _, ids := range idGroups {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}

	return result
}

// buildFeedResponse 构建单个Feed响应
func (s *FeedService) buildFeedResponse(feed *models.Feed, currentUserID uint) *models.FeedResponse {
	author, _ := s.userService.GetUserByID(feed.UserID)

	resp := &models.FeedResponse{
		ID:           feed.ID,
		UserID:       feed.UserID,
		Content:      feed.Content,
		Images:       feed.Images,
		Videos:       feed.Videos,
		FeedType:     feed.FeedType,
		OriginalID:   feed.OriginalID,
		LikeCount:    feed.LikeCount,
		CommentCount: feed.CommentCount,
		ShareCount:   feed.ShareCount,
		CreatedAt:    feed.CreatedAt,
		UpdatedAt:    feed.UpdatedAt,
	}

	if author != nil {
		resp.Author = author.ToResponse()
	}

	// 如果是转发，查询原始Feed
	if feed.FeedType == models.FeedTypeRepost && feed.OriginalID != nil {
		var originalFeed models.Feed
		if err := models.DB.Where("id = ?", *feed.OriginalID).First(&originalFeed).Error; err == nil {
			origResp := s.buildOriginalFeedResponse(&originalFeed)
			resp.OriginalFeed = origResp
		}
	}

	// 检查是否点赞
	if currentUserID > 0 {
		var count int64
		models.DB.Model(&models.Like{}).
			Where("user_id = ? AND feed_id = ?", currentUserID, feed.ID).
			Count(&count)
		resp.IsLiked = count > 0
	}

	return resp
}

// buildOriginalFeedResponse 构建原始Feed响应（转发时展示，不递归转发链）
func (s *FeedService) buildOriginalFeedResponse(feed *models.Feed) *models.FeedResponse {
	author, _ := s.userService.GetUserByID(feed.UserID)

	resp := &models.FeedResponse{
		ID:           feed.ID,
		UserID:       feed.UserID,
		Content:      feed.Content,
		Images:       feed.Images,
		Videos:       feed.Videos,
		FeedType:     feed.FeedType,
		LikeCount:    feed.LikeCount,
		CommentCount: feed.CommentCount,
		ShareCount:   feed.ShareCount,
		CreatedAt:    feed.CreatedAt,
		UpdatedAt:    feed.UpdatedAt,
	}

	if author != nil {
		resp.Author = author.ToResponse()
	}

	return resp
}

// buildFeedResponses 批量构建Feed响应
func (s *FeedService) buildFeedResponses(feeds []models.Feed, currentUserID uint) []models.FeedResponse {
	if len(feeds) == 0 {
		return []models.FeedResponse{}
	}

	// 批量查询作者信息
	userIDs := make([]uint, 0)
	userMap := make(map[uint]models.User)
	for _, feed := range feeds {
		userIDs = append(userIDs, feed.UserID)
	}

	var users []models.User
	models.DB.Where("id IN ?", userIDs).Find(&users)
	for _, user := range users {
		userMap[user.ID] = user
	}

	// 批量查询点赞状态
	likedMap := make(map[uint]bool)
	if currentUserID > 0 {
		feedIDs := make([]uint, 0)
		for _, feed := range feeds {
			feedIDs = append(feedIDs, feed.ID)
		}

		var likes []models.Like
		models.DB.Where("user_id = ? AND feed_id IN ?", currentUserID, feedIDs).Find(&likes)
		for _, like := range likes {
			likedMap[like.FeedID] = true
		}
	}

	// 批量查询原始Feed（用于转发）
	originalIDs := make([]uint, 0)
	for _, feed := range feeds {
		if feed.FeedType == models.FeedTypeRepost && feed.OriginalID != nil {
			originalIDs = append(originalIDs, *feed.OriginalID)
		}
	}
	originalMap := make(map[uint]models.Feed)
	if len(originalIDs) > 0 {
		var originals []models.Feed
		models.DB.Where("id IN ?", originalIDs).Find(&originals)
		for _, orig := range originals {
			originalMap[orig.ID] = orig
		}
		// 查询原始Feed的作者
		origUserIDs := make([]uint, 0)
		for _, orig := range originals {
			origUserIDs = append(origUserIDs, orig.UserID)
		}
		if len(origUserIDs) > 0 {
			var origUsers []models.User
			models.DB.Where("id IN ?", origUserIDs).Find(&origUsers)
			for _, u := range origUsers {
				userMap[u.ID] = u
			}
		}
	}

	// 构建响应
	responses := make([]models.FeedResponse, 0, len(feeds))
	for _, feed := range feeds {
		resp := models.FeedResponse{
			ID:           feed.ID,
			UserID:       feed.UserID,
			Content:      feed.Content,
			Images:       feed.Images,
			Videos:       feed.Videos,
			FeedType:     feed.FeedType,
			OriginalID:   feed.OriginalID,
			LikeCount:    feed.LikeCount,
			CommentCount: feed.CommentCount,
			ShareCount:   feed.ShareCount,
			CreatedAt:    feed.CreatedAt,
			UpdatedAt:    feed.UpdatedAt,
			IsLiked:      likedMap[feed.ID],
		}

		if author, ok := userMap[feed.UserID]; ok {
			resp.Author = author.ToResponse()
		}

		// 构建转发的原始Feed
		if feed.FeedType == models.FeedTypeRepost && feed.OriginalID != nil {
			if orig, ok := originalMap[*feed.OriginalID]; ok {
				origResp := &models.FeedResponse{
					ID:           orig.ID,
					UserID:       orig.UserID,
					Content:      orig.Content,
					Images:       orig.Images,
					Videos:       orig.Videos,
					FeedType:     orig.FeedType,
					LikeCount:    orig.LikeCount,
					CommentCount: orig.CommentCount,
					ShareCount:   orig.ShareCount,
					CreatedAt:    orig.CreatedAt,
				}
				if origAuthor, ok2 := userMap[orig.UserID]; ok2 {
					origResp.Author = origAuthor.ToResponse()
				}
				resp.OriginalFeed = origResp
			}
		}

		responses = append(responses, resp)
	}

	return responses
}

// LikeFeed 点赞
func (s *FeedService) LikeFeed(userID, feedID uint) error {
	var feed models.Feed
	if err := models.DB.Where("id = ?", feedID).First(&feed).Error; err != nil {
		return errors.New("动态不存在")
	}

	var count int64
	models.DB.Model(&models.Like{}).
		Where("user_id = ? AND feed_id = ?", userID, feedID).
		Count(&count)
	if count > 0 {
		return errors.New("已点赞")
	}

	tx := models.DB.Begin()

	like := &models.Like{
		UserID:    userID,
		FeedID:    feedID,
		CreatedAt: time.Now(),
	}
	if err := tx.Create(like).Error; err != nil {
		tx.Rollback()
		return errors.New("点赞失败")
	}

	if err := tx.Model(&models.Feed{}).Where("id = ?", feedID).
		UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error; err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

// UnlikeFeed 取消点赞
func (s *FeedService) UnlikeFeed(userID, feedID uint) error {
	var like models.Like
	if err := models.DB.Where("user_id = ? AND feed_id = ?", userID, feedID).
		First(&like).Error; err != nil {
		return errors.New("未点赞")
	}

	tx := models.DB.Begin()

	if err := tx.Delete(&like).Error; err != nil {
		tx.Rollback()
		return errors.New("取消点赞失败")
	}

	if err := tx.Model(&models.Feed{}).Where("id = ?", feedID).
		UpdateColumn("like_count", gorm.Expr("GREATEST(like_count - 1, 0)")).Error; err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

// CommentFeed 评论Feed
func (s *FeedService) CommentFeed(userID, feedID uint, content string) (*models.Comment, error) {
	var feed models.Feed
	if err := models.DB.Where("id = ?", feedID).First(&feed).Error; err != nil {
		return nil, errors.New("动态不存在")
	}

	tx := models.DB.Begin()

	comment := &models.Comment{
		UserID:  userID,
		FeedID:  feedID,
		Content: content,
	}

	if err := tx.Create(comment).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("评论失败")
	}

	if err := tx.Model(&models.Feed{}).Where("id = ?", feedID).
		UpdateColumn("comment_count", gorm.Expr("comment_count + 1")).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	tx.Commit()
	return comment, nil
}

// GetComments 获取评论列表
func (s *FeedService) GetComments(feedID uint, page, pageSize int) ([]map[string]interface{}, int64, error) {
	var comments []models.Comment
	var total int64

	query := models.DB.Model(&models.Comment{}).Where("feed_id = ?", feedID)
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at ASC").Offset(offset).Limit(pageSize).Find(&comments).Error; err != nil {
		return nil, 0, err
	}

	userIDs := make([]uint, 0)
	for _, c := range comments {
		userIDs = append(userIDs, c.UserID)
	}

	userMap := make(map[uint]models.User)
	if len(userIDs) > 0 {
		var users []models.User
		models.DB.Where("id IN ?", userIDs).Find(&users)
		for _, user := range users {
			userMap[user.ID] = user
		}
	}

	var result []map[string]interface{}
	for _, c := range comments {
		item := map[string]interface{}{
			"id":         c.ID,
			"user_id":    c.UserID,
			"content":    c.Content,
			"created_at": c.CreatedAt,
		}
		if author, ok := userMap[c.UserID]; ok {
			item["author"] = author.ToResponse()
		}
		result = append(result, item)
	}

	return result, total, nil
}

// DeleteComment 删除评论（仅评论作者可删除）
func (s *FeedService) DeleteComment(currentUserID, feedID, commentID uint) error {
	var comment models.Comment
	if err := models.DB.Where("id = ? AND feed_id = ?", commentID, feedID).First(&comment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("评论不存在")
		}
		return err
	}

	if comment.UserID != currentUserID {
		return errors.New("无权限删除该评论")
	}

	tx := models.DB.Begin()

	if err := tx.Delete(&comment).Error; err != nil {
		tx.Rollback()
		return errors.New("删除评论失败")
	}

	if err := tx.Model(&models.Feed{}).Where("id = ?", feedID).
		UpdateColumn("comment_count", gorm.Expr("GREATEST(comment_count - 1, 0)")).Error; err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

// GetFeedLikers 获取点赞用户列表
func (s *FeedService) GetFeedLikers(feedID uint, page, pageSize int) ([]models.UserResponse, int64, error) {
	var likes []models.Like
	var total int64

	query := models.DB.Model(&models.Like{}).Where("feed_id = ?", feedID)
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&likes).Error; err != nil {
		return nil, 0, err
	}

	if len(likes) == 0 {
		return []models.UserResponse{}, total, nil
	}

	userIDs := make([]uint, 0, len(likes))
	for _, like := range likes {
		userIDs = append(userIDs, like.UserID)
	}

	var users []models.User
	if err := models.DB.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	userMap := make(map[uint]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	result := make([]models.UserResponse, 0, len(likes))
	for _, like := range likes {
		if u, ok := userMap[like.UserID]; ok {
			result = append(result, u.ToResponse())
		}
	}

	return result, total, nil
}
