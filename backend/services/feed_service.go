package services

import (
	"encoding/json"
	"errors"
	"feed/cache"
	"feed/models"
	"feed/mq"
	"feed/repository"
	"log"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// FeedService 负责动态域的业务编排：发布、时间线聚合、互动（赞/评/转）等。
// 说明：
// 1) service 层只做业务规则与流程控制；
// 2) 数据访问下沉到 repository；
// 3) 分发相关能力委托给 mq/cache。
type FeedService struct {
	userService         *UserService
	feedRepo            repository.FeedRepository
	userRepo            repository.UserRepository
	followRepo          repository.FollowRepository
	notificationService *NotificationService
}

// NewFeedService 构建 FeedService。
func NewFeedService() *FeedService {
	return &FeedService{
		userService:         &UserService{},
		feedRepo:            repository.NewFeedRepository(models.DB),
		userRepo:            repository.NewUserRepository(models.DB),
		followRepo:          repository.NewFollowRepository(models.DB),
		notificationService: NewNotificationService(),
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
	Content    string `json:"content"`
	OriginalID uint   `json:"original_id" binding:"required"`
}

// PublishFeed 发布动态：
// 1) 先做参数约束（文案/图片/视频至少其一）；
// 2) 写入 feed 主表；
// 3) 投递 MQ 走异步分发。
func (s *FeedService) PublishFeed(userID uint, req *CreateFeedRequest) (*models.Feed, error) {
	if req.Content == "" && req.Images == "" && req.Videos == "" {
		return nil, errors.New("请输入文案、上传图片或视频")
	}

	feed := &models.Feed{UserID: userID, Content: req.Content, Images: req.Images, Videos: req.Videos, FeedType: models.FeedTypeOriginal}
	if err := s.feedRepo.Create(feed); err != nil {
		return nil, errors.New("发布失败")
	}
	cache.AddFeedID(feed.ID)
	mq.PublishFeed(feed.ID, userID)
	return feed, nil
}

func (s *FeedService) UpdateFeed(feedID, userID uint, req *UpdateFeedRequest) (*models.Feed, error) {
	feed, err := s.feedRepo.GetByIDAndUserID(feedID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("动态不存在或无权限编辑")
		}
		return nil, err
	}

	updates := make(map[string]any)
	if req.Content != "" {
		updates["content"] = req.Content
	}
	if feed.FeedType == models.FeedTypeOriginal {
		if req.Images != "" {
			updates["images"] = req.Images
		}
		if req.Videos != "" {
			updates["videos"] = req.Videos
		}
	}
	if len(updates) == 0 {
		return nil, errors.New("请至少填写一个要更新的字段")
	}
	if err := s.feedRepo.UpdateByID(feedID, updates); err != nil {
		return nil, errors.New("编辑失败")
	}
	_ = cache.RedisClient.Del(cache.Ctx, "feed:"+strconv.FormatUint(uint64(feedID), 10)).Err()
	_ = cache.DeleteFeedDetail(feedID)
	return s.feedRepo.GetByID(feedID)
}

func (s *FeedService) RepostFeed(userID uint, req *RepostFeedRequest) (*models.Feed, error) {
	originalFeed, err := s.feedRepo.GetByID(req.OriginalID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("原始动态不存在")
		}
		return nil, err
	}

	originalID := req.OriginalID
	if originalFeed.FeedType == models.FeedTypeRepost && originalFeed.OriginalID != nil {
		originalID = *originalFeed.OriginalID
	}

	feed := &models.Feed{UserID: userID, Content: req.Content, FeedType: models.FeedTypeRepost, OriginalID: &originalID}
	tx := s.feedRepo.BeginTx()
	if err := tx.Create(feed).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("转发失败")
	}
	if err := s.feedRepo.IncreaseShareCount(tx, originalID); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("转发失败")
	}
	cache.AddFeedID(feed.ID)
	mq.PublishFeed(feed.ID, userID)
	return feed, nil
}

func (s *FeedService) DeleteFeed(feedID, userID uint) error {
	feed, err := s.feedRepo.GetByIDAndUserID(feedID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("动态不存在或无权限删除")
		}
		return err
	}

	if err := s.feedRepo.Delete(feed); err != nil {
		return errors.New("删除失败")
	}
	_ = cache.RedisClient.Del(cache.Ctx, "feed:"+strconv.FormatUint(uint64(feedID), 10)).Err()

	key := strconv.FormatUint(uint64(feedID), 10)
	_ = cache.RedisClient.ZRem(cache.Ctx, "outbox:"+strconv.FormatUint(uint64(userID), 10), key).Err()

	go func() {
		timelines, _ := s.feedRepo.ListTimelinesByFeedID(feedID)
		for _, tl := range timelines {
			cache.RemoveFromInbox(tl.UserID, feedID)
		}
		_ = s.feedRepo.DeleteTimelineByFeedID(feedID)
	}()

	return nil
}

// GetFeedByID 获取动态详情：优先读缓存，缓存未命中时 setnx回源请求。
func (s *FeedService) GetFeedByID(feedID, currentUserID uint) (*models.FeedResponse, error) {
	if !cache.MightFeedExist(feedID) {
		return nil, errors.New("动态不存在")
	}

	if raw, err := cache.GetFeedDetail(feedID); err == nil && raw != "" {
		var resp models.FeedResponse
		if unmarshalErr := json.Unmarshal([]byte(raw), &resp); unmarshalErr == nil {
			if currentUserID > 0 {
				if like, likeErr := s.feedRepo.GetLike(currentUserID, feedID); likeErr == nil && like != nil {
					resp.IsLiked = true
				}
			}
			return &resp, nil
		}
	}
	//setnx 获取锁 回源请求
	lockKey := "lock:feed_detail:" + strconv.FormatUint(uint64(feedID), 10)
	lockToken := strconv.FormatInt(time.Now().UnixNano(), 10)
	locked, _ := cache.AcquireLock(lockKey, lockToken, 3*time.Second)
	if !locked {
		for i := 0; i < 8; i++ {
			time.Sleep(40 * time.Millisecond)
			if raw, err := cache.GetFeedDetail(feedID); err == nil && raw != "" {
				var resp models.FeedResponse
				if unmarshalErr := json.Unmarshal([]byte(raw), &resp); unmarshalErr == nil {
					if currentUserID > 0 {
						if like, likeErr := s.feedRepo.GetLike(currentUserID, feedID); likeErr == nil && like != nil {
							resp.IsLiked = true
						}
					}
					return &resp, nil
				}
			}
		}
	}
	if locked {
		defer func() { _ = cache.ReleaseLock(lockKey, lockToken) }()
	}

	feed, dbErr := s.feedRepo.GetByID(feedID)
	if dbErr != nil {
		if errors.Is(dbErr, gorm.ErrRecordNotFound) {
			return nil, errors.New("动态不存在")
		}
		return nil, dbErr
	}
	resp := s.buildFeedResponse(feed, currentUserID)
	cachePayload := *resp
	cachePayload.IsLiked = false
	if b, mErr := json.Marshal(cachePayload); mErr == nil {
		_ = cache.CacheFeedDetail(feedID, string(b))
	}
	return resp, nil
}

func (s *FeedService) GetUserFeeds(userID uint, page, pageSize int, currentUserID uint) ([]models.FeedResponse, int64, error) {
	feeds, total, err := s.feedRepo.ListByUserID(userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	responses := s.buildFeedResponses(feeds, currentUserID)
	return responses, total, nil
}

// GetTimeline 获取时间线（真正游标分页）。
// cursor 语义："created_at_unix_nano|feed_id"，首次传空字符串。
// 返回 nextCursor 继续向更旧内容翻页。
func (s *FeedService) GetTimelineByCursor(userID uint, cursor string, pageSize int) ([]models.FeedResponse, string, bool, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	//解析游标
	cursorTime, cursorID := parseTimelineCursor(cursor)
	prefetchSize := pageSize + 1
	candidateIDs := make([]uint, 0, prefetchSize*8)
	//拉取自己的动态
	ownFeeds, _ := s.feedRepo.ListRecentByUserID(userID, prefetchSize*4)
	candidateIDs = append(candidateIDs, s.filterTimelineCandidates(ownFeeds, cursorTime, cursorID)...)
	//拉取收件箱
	if inboxIDs, err := cache.GetInboxByScore(userID, -1, float64(cursorTime.UnixNano()), int64(prefetchSize*6)); err == nil {
		for _, idStr := range inboxIDs {
			id, _ := strconv.ParseUint(idStr, 10, 64)
			if id > 0 {
				candidateIDs = append(candidateIDs, uint(id))
			}
		}
	} else {
		log.Printf("Get inbox from redis failed: %v", err)
	}
	//拉取大V的发件箱
	candidateIDs = append(candidateIDs, s.pullBigVFeeds(userID, int64(prefetchSize*2), cursorTime)...)
	candidateIDs = s.uniqueUint(candidateIDs)
	if len(candidateIDs) == 0 {
		return []models.FeedResponse{}, cursor, false, nil
	}
	//拉取动态
	feeds, err := s.feedRepo.ListByIDsBeforeCursor(candidateIDs, cursorTime, cursorID)
	if err != nil {
		return nil, cursor, false, err
	}
	//分页
	pageFeeds, hasMore := pickTimelinePage(feeds, pageSize)
	if len(pageFeeds) == 0 {
		return []models.FeedResponse{}, cursor, false, nil
	}

	responses := s.buildFeedResponses(pageFeeds, userID)
	last := pageFeeds[len(pageFeeds)-1]
	//构建游标
	nextCursor := buildTimelineCursor(last.CreatedAt, last.ID)
	if len(feeds) > len(pageFeeds) {
		hasMore = true
	}
	//判断是否还有更多
	if !hasMore && len(pageFeeds) == pageSize {
		hasMore = s.hasMoreTimelineAfter(userID, last.CreatedAt, last.ID)
	}
	return responses, nextCursor, hasMore, nil
}

// 判断是否还有更多
func (s *FeedService) hasMoreTimelineAfter(userID uint, lastTime time.Time, lastID uint) bool {
	ownFeeds, _ := s.feedRepo.ListRecentByUserID(userID, 100)        //拉取自己的动态
	ownIDs := s.filterTimelineCandidates(ownFeeds, lastTime, lastID) //过滤动态
	if len(ownIDs) > 0 {
		return true
	}
	//拉取收件箱
	inboxIDs, err := cache.GetInboxByScore(userID, -1, float64(lastTime.UnixNano()), 100)
	if err != nil || len(inboxIDs) == 0 {
		return false
	}

	ids := make([]uint, 0, len(inboxIDs))
	for _, idStr := range inboxIDs {
		id, _ := strconv.ParseUint(idStr, 10, 64)
		if id > 0 {
			ids = append(ids, uint(id))
		}
	}
	//拉取动态
	feeds, err := s.feedRepo.ListByIDsNewerThanCursor(ids, lastTime, lastID)
	if err != nil {
		return false
	}
	return len(feeds) > 0
}

// pullBigVFeeds 拉取当前用户所关注大V的发件箱内容。
// 若 Redis 不可用则回源 DB 最近动态，保证可用性优先。
// pullBigVFeeds 拉取当前用户关注的大V发件箱动态。
// 若 Redis outbox 缺失，则回源数据库兜底。
func (s *FeedService) pullBigVFeeds(userID uint, limit int64, cursorTime time.Time) []uint {
	follows, _ := s.followRepo.ListFollowingAll(userID)
	seen := make(map[uint]bool)
	pullFeedIDs := make([]uint, 0, len(follows)*int(limit))
	cursorNano := float64(cursorTime.UnixNano())

	for _, follow := range follows {
		isBigV, err := cache.IsBigV(follow.FollowedID)
		if err != nil {
			if user, dbErr := s.userRepo.GetByID(follow.FollowedID); dbErr == nil {
				isBigV = user.IsBigV
			}
		}
		if !isBigV {
			continue
		}

		appendFeedID := func(id uint) {
			if id == 0 || seen[id] {
				return
			}
			seen[id] = true
			pullFeedIDs = append(pullFeedIDs, id)
		}

		outboxIDs, err := cache.GetOutboxByScore(follow.FollowedID, -1, cursorNano, limit)
		if err != nil || len(outboxIDs) == 0 {
			feeds, _ := s.feedRepo.ListRecentByUserID(follow.FollowedID, int(limit))
			for _, feed := range feeds {
				if feed.CreatedAt.After(cursorTime) || feed.CreatedAt.Equal(cursorTime) {
					continue
				}
				appendFeedID(feed.ID)
			}
			continue
		}
		for _, idStr := range outboxIDs {
			id, _ := strconv.ParseUint(idStr, 10, 64)
			appendFeedID(uint(id))
		}
	}
	return pullFeedIDs
}

// 合并并去重
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
// 去重
func (s *FeedService) uniqueUint(values []uint) []uint {
	seen := make(map[uint]bool)
	result := make([]uint, 0, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func (s *FeedService) buildFeedResponse(feed *models.Feed, currentUserID uint) *models.FeedResponse {
	author, _ := s.userRepo.GetByID(feed.UserID)
	resp := &models.FeedResponse{ID: feed.ID, UserID: feed.UserID, Content: feed.Content, Images: feed.Images, Videos: feed.Videos, FeedType: feed.FeedType, OriginalID: feed.OriginalID, LikeCount: feed.LikeCount, CommentCount: feed.CommentCount, ShareCount: feed.ShareCount, CreatedAt: feed.CreatedAt, UpdatedAt: feed.UpdatedAt}
	if author != nil {
		resp.Author = author.ToResponse()
	}

	if feed.FeedType == models.FeedTypeRepost && feed.OriginalID != nil {
		if originalFeed, err := s.feedRepo.GetByID(*feed.OriginalID); err == nil {
			origResp := s.buildOriginalFeedResponse(originalFeed)
			resp.OriginalFeed = origResp
		}
	}

	if currentUserID > 0 {
		if like, err := s.feedRepo.GetLike(currentUserID, feed.ID); err == nil && like != nil {
			resp.IsLiked = true
		}
	}
	return resp
}

func (s *FeedService) buildOriginalFeedResponse(feed *models.Feed) *models.FeedResponse {
	author, _ := s.userRepo.GetByID(feed.UserID)
	resp := &models.FeedResponse{ID: feed.ID, UserID: feed.UserID, Content: feed.Content, Images: feed.Images, Videos: feed.Videos, FeedType: feed.FeedType, LikeCount: feed.LikeCount, CommentCount: feed.CommentCount, ShareCount: feed.ShareCount, CreatedAt: feed.CreatedAt, UpdatedAt: feed.UpdatedAt}
	if author != nil {
		resp.Author = author.ToResponse()
	}
	return resp
}

// buildFeedResponses 批量构建动态响应，减少 N+1 查询：
// 1) 批量加载作者
// 2) 批量加载当前用户点赞状态
// 3) 批量加载转发原文与原作者
func (s *FeedService) buildFeedResponses(feeds []models.Feed, currentUserID uint) []models.FeedResponse {
	if len(feeds) == 0 {
		return []models.FeedResponse{}
	}

	userIDs := make([]uint, 0, len(feeds))
	for _, feed := range feeds {
		userIDs = append(userIDs, feed.UserID)
	}

	feedIDs := make([]uint, 0, len(feeds))
	for _, feed := range feeds {
		feedIDs = append(feedIDs, feed.ID)
	}

	originalIDs := make([]uint, 0)
	for _, feed := range feeds {
		if feed.FeedType == models.FeedTypeRepost && feed.OriginalID != nil {
			originalIDs = append(originalIDs, *feed.OriginalID)
		}
	}

	originals := make([]models.Feed, 0)
	if len(originalIDs) > 0 {
		originals, _ = s.feedRepo.ListByIDs(s.uniqueUint(originalIDs))
		for _, orig := range originals {
			userIDs = append(userIDs, orig.UserID)
		}
	}

	userIDs = s.uniqueUint(userIDs)
	users, _ := s.userRepo.ListByIDs(userIDs)
	userMap := map[uint]models.User{}
	for _, u := range users {
		userMap[u.ID] = u
	}

	likedMap := map[uint]bool{}
	if currentUserID > 0 {
		likes, _ := s.feedRepo.ListLikesByUserAndFeedIDs(currentUserID, feedIDs)
		for _, like := range likes {
			likedMap[like.FeedID] = true
		}
	}

	originalMap := map[uint]models.Feed{}
	for _, orig := range originals {
		originalMap[orig.ID] = orig
	}

	responses := make([]models.FeedResponse, 0, len(feeds))
	for _, feed := range feeds {
		resp := models.FeedResponse{ID: feed.ID, UserID: feed.UserID, Content: feed.Content, Images: feed.Images, Videos: feed.Videos, FeedType: feed.FeedType, OriginalID: feed.OriginalID, LikeCount: feed.LikeCount, CommentCount: feed.CommentCount, ShareCount: feed.ShareCount, CreatedAt: feed.CreatedAt, UpdatedAt: feed.UpdatedAt, IsLiked: likedMap[feed.ID]}
		if author, ok := userMap[feed.UserID]; ok {
			resp.Author = author.ToResponse()
		}
		if feed.FeedType == models.FeedTypeRepost && feed.OriginalID != nil {
			if orig, ok := originalMap[*feed.OriginalID]; ok {
				origResp := &models.FeedResponse{ID: orig.ID, UserID: orig.UserID, Content: orig.Content, Images: orig.Images, Videos: orig.Videos, FeedType: orig.FeedType, LikeCount: orig.LikeCount, CommentCount: orig.CommentCount, ShareCount: orig.ShareCount, CreatedAt: orig.CreatedAt}
				if oa, ok2 := userMap[orig.UserID]; ok2 {
					origResp.Author = oa.ToResponse()
				}
				resp.OriginalFeed = origResp
			}
		}
		responses = append(responses, resp)
	}
	return responses
}

// LikeFeed 点赞动态，并在成功后失效该动态详情缓存，避免计数陈旧。
func (s *FeedService) LikeFeed(userID, feedID uint) error {
	if _, err := s.feedRepo.GetByID(feedID); err != nil {
		return errors.New("动态不存在")
	}
	if like, err := s.feedRepo.GetLike(userID, feedID); err == nil && like != nil {
		return errors.New("已点赞")
	}
	tx := s.feedRepo.BeginTx()
	like := &models.Like{UserID: userID, FeedID: feedID, CreatedAt: time.Now()}
	if err := s.feedRepo.CreateLike(tx, like); err != nil {
		tx.Rollback()
		return errors.New("点赞失败")
	}
	if err := s.feedRepo.IncreaseLikeCount(tx, feedID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return errors.New("点赞失败")
	}
	_ = cache.DeleteFeedDetail(feedID)
	if feed, err := s.feedRepo.GetByID(feedID); err == nil {
		s.notificationService.CreateLikeNotification(userID, feed.UserID, feedID)
	}
	return nil
}

// UnlikeFeed 取消点赞，并失效动态详情缓存，保证读到最新计数。
func (s *FeedService) UnlikeFeed(userID, feedID uint) error {
	like, err := s.feedRepo.GetLike(userID, feedID)
	if err != nil || like == nil {
		return errors.New("未点赞")
	}
	tx := s.feedRepo.BeginTx()
	if err := s.feedRepo.DeleteLike(tx, like); err != nil {
		tx.Rollback()
		return errors.New("取消点赞失败")
	}
	if err := s.feedRepo.DecreaseLikeCount(tx, feedID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return errors.New("取消点赞失败")
	}
	_ = cache.DeleteFeedDetail(feedID)
	return nil
}

// CommentFeed 发布评论，并失效动态详情缓存，保证评论数及时刷新。
func (s *FeedService) CommentFeed(userID, feedID uint, content string) (*models.Comment, error) {
	if _, err := s.feedRepo.GetByID(feedID); err != nil {
		return nil, errors.New("动态不存在")
	}
	tx := s.feedRepo.BeginTx()
	comment := &models.Comment{UserID: userID, FeedID: feedID, Content: content}
	if err := s.feedRepo.CreateComment(tx, comment); err != nil {
		tx.Rollback()
		return nil, errors.New("评论失败")
	}
	if err := s.feedRepo.IncreaseCommentCount(tx, feedID); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("评论失败")
	}
	_ = cache.DeleteFeedDetail(feedID)
	if feed, err := s.feedRepo.GetByID(feedID); err == nil {
		s.notificationService.CreateCommentNotification(userID, feed.UserID, feedID, content)
	}
	return comment, nil
}

func (s *FeedService) GetComments(feedID uint, page, pageSize int) ([]map[string]interface{}, int64, error) {
	comments, total, err := s.feedRepo.ListCommentsByFeedID(feedID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	userIDs := make([]uint, 0, len(comments))
	for _, c := range comments {
		userIDs = append(userIDs, c.UserID)
	}
	users, _ := s.userRepo.ListByIDs(userIDs)
	userMap := map[uint]models.User{}
	for _, u := range users {
		userMap[u.ID] = u
	}
	result := make([]map[string]interface{}, 0, len(comments))
	for _, c := range comments {
		item := map[string]interface{}{"id": c.ID, "user_id": c.UserID, "content": c.Content, "created_at": c.CreatedAt}
		if author, ok := userMap[c.UserID]; ok {
			item["author"] = author.ToResponse()
		}
		result = append(result, item)
	}
	return result, total, nil
}

// DeleteComment 删除评论，并失效动态详情缓存，避免评论数不一致。
func (s *FeedService) DeleteComment(currentUserID, feedID, commentID uint) error {
	comment, err := s.feedRepo.GetCommentByIDAndFeedID(commentID, feedID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("评论不存在")
		}
		return err
	}
	if comment.UserID != currentUserID {
		return errors.New("无权限删除该评论")
	}
	tx := s.feedRepo.BeginTx()
	if err := s.feedRepo.DeleteComment(tx, comment); err != nil {
		tx.Rollback()
		return errors.New("删除评论失败")
	}
	if err := s.feedRepo.DecreaseCommentCount(tx, feedID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return errors.New("删除评论失败")
	}
	_ = cache.DeleteFeedDetail(feedID)
	return nil
}

func (s *FeedService) GetFeedLikers(feedID uint, page, pageSize int) ([]models.UserResponse, int64, error) {
	likes, total, err := s.feedRepo.ListLikesByFeedID(feedID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	if len(likes) == 0 {
		return []models.UserResponse{}, total, nil
	}
	userIDs := make([]uint, 0, len(likes))
	for _, like := range likes {
		userIDs = append(userIDs, like.UserID)
	}
	users, err := s.userRepo.ListByIDs(userIDs)
	if err != nil {
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

// SearchFeeds 搜索动态内容，用于全局搜索“动态”tab。
func (s *FeedService) SearchFeeds(keyword string, page, pageSize int, currentUserID uint) ([]models.FeedResponse, int64, error) {
	feeds, total, err := s.feedRepo.SearchByKeyword(keyword, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return s.buildFeedResponses(feeds, currentUserID), total, nil
}

func parseTimelineCursor(cursor string) (time.Time, uint) {
	maxTimelineTime := time.Date(9999, 12, 30, 23, 59, 59, 999999999, time.UTC)
	if strings.TrimSpace(cursor) == "" {
		return maxTimelineTime, ^uint(0)
	}
	parts := strings.Split(cursor, "|")
	if len(parts) != 2 {
		return maxTimelineTime, ^uint(0)
	}
	nano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return maxTimelineTime, ^uint(0)
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return maxTimelineTime, ^uint(0)
	}
	return time.Unix(0, nano), uint(id)
}
func buildTimelineCursor(createdAt time.Time, id uint) string {
	if createdAt.IsZero() {
		createdAt = time.Date(9999, 12, 30, 23, 59, 59, 999999999, time.UTC)
	}
	return strconv.FormatInt(createdAt.UnixNano(), 10) + "|" + strconv.FormatUint(uint64(id), 10)
}

func (s *FeedService) filterTimelineCandidates(feeds []models.Feed, cursorTime time.Time, cursorID uint) []uint {
	ids := make([]uint, 0, len(feeds))
	for _, feed := range feeds {
		//如果动态创建时间大于游标时间 或者 动态创建时间等于游标时间且动态ID大于等于游标ID 则跳过
		if feed.CreatedAt.After(cursorTime) || (feed.CreatedAt.Equal(cursorTime) && feed.ID >= cursorID) {
			continue
		}
		ids = append(ids, feed.ID)
	}
	return ids
}

func pickTimelinePage(feeds []models.Feed, pageSize int) ([]models.Feed, bool) {
	//如果动态为空 返回空数组和false
	if len(feeds) == 0 {
		return []models.Feed{}, false
	}
	//如果动态数量小于等于分页大小 返回动态和false
	if len(feeds) <= pageSize {
		return feeds, false
	}
	return feeds[:pageSize], true
}
