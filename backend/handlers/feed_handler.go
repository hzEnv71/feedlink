package handlers

import (
	"feed/middleware"
	"feed/services"
	"feed/utils"
	"strconv"

	"github.com/gin-gonic/gin"
)

type FeedHandler struct {
	feedService *services.FeedService
}

func NewFeedHandler() *FeedHandler {
	return &FeedHandler{
		feedService: services.NewFeedService(),
	}
}

// PublishFeed 发布动态（支持文案+图片+视频）
// POST /api/feeds
func (h *FeedHandler) PublishFeed(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)

	var req services.CreateFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	feed, err := h.feedService.PublishFeed(currentUserID, &req)
	if err != nil {
		utils.Error(c, 500, err.Error())
		return
	}

	resp, _ := h.feedService.GetFeedByID(feed.ID, currentUserID)
	utils.SuccessWithMessage(c, "发布成功", resp)
}

// UpdateFeed 编辑动态
// PUT /api/feeds/:id
func (h *FeedHandler) UpdateFeed(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)
	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	var req services.UpdateFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	_, err = h.feedService.UpdateFeed(uint(feedID), currentUserID, &req)
	if err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	resp, _ := h.feedService.GetFeedByID(uint(feedID), currentUserID)
	utils.SuccessWithMessage(c, "编辑成功", resp)
}

// RepostFeed 转发动态
// POST /api/feeds/repost
func (h *FeedHandler) RepostFeed(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)

	var req services.RepostFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	feed, err := h.feedService.RepostFeed(currentUserID, &req)
	if err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	resp, _ := h.feedService.GetFeedByID(feed.ID, currentUserID)
	utils.SuccessWithMessage(c, "转发成功", resp)
}

// DeleteFeed 删除动态
// DELETE /api/feeds/:id
func (h *FeedHandler) DeleteFeed(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)
	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	if err := h.feedService.DeleteFeed(uint(feedID), currentUserID); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	utils.SuccessWithMessage(c, "删除成功", nil)
}

// GetFeed 获取动态详情
// GET /api/feeds/:id
func (h *FeedHandler) GetFeed(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)
	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	resp, err := h.feedService.GetFeedByID(uint(feedID), currentUserID)
	if err != nil {
		utils.Error(c, 404, err.Error())
		return
	}

	utils.Success(c, resp)
}

// GetUserFeeds 获取用户动态列表
// GET /api/users/:id/feeds?page=1&page_size=20
func (h *FeedHandler) GetUserFeeds(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)
	userIDStr := c.Param("id")
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "用户ID无效")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	feeds, total, err := h.feedService.GetUserFeeds(uint(userID), page, pageSize, currentUserID)
	if err != nil {
		utils.Error(c, 500, "获取动态列表失败")
		return
	}

	utils.SuccessPage(c, feeds, total, page, pageSize)
}

// GetTimeline 获取时间线（Feed流）- 展示本人和关注的用户的Feed
// GET /api/timeline?page=1&page_size=20
func (h *FeedHandler) GetTimeline(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	feeds, total, err := h.feedService.GetTimeline(currentUserID, page, pageSize)
	if err != nil {
		utils.Error(c, 500, "获取时间线失败")
		return
	}

	utils.SuccessPage(c, feeds, total, page, pageSize)
}

// LikeFeed 点赞
// POST /api/feeds/:id/like
func (h *FeedHandler) LikeFeed(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)
	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	if err := h.feedService.LikeFeed(currentUserID, uint(feedID)); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	utils.SuccessWithMessage(c, "点赞成功", nil)
}

// UnlikeFeed 取消点赞
// DELETE /api/feeds/:id/like
func (h *FeedHandler) UnlikeFeed(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)
	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	if err := h.feedService.UnlikeFeed(currentUserID, uint(feedID)); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	utils.SuccessWithMessage(c, "取消点赞成功", nil)
}

// CommentFeed 评论动态
// POST /api/feeds/:id/comments
func (h *FeedHandler) CommentFeed(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)
	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	var req struct {
		Content string `json:"content" binding:"required,min=1,max=500"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	comment, err := h.feedService.CommentFeed(currentUserID, uint(feedID), req.Content)
	if err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	utils.SuccessWithMessage(c, "评论成功", comment)
}

// GetComments 获取评论列表
// GET /api/feeds/:id/comments?page=1&page_size=20
func (h *FeedHandler) GetComments(c *gin.Context) {
	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	comments, total, err := h.feedService.GetComments(uint(feedID), page, pageSize)
	if err != nil {
		utils.Error(c, 500, "获取评论列表失败")
		return
	}

	utils.SuccessPage(c, comments, total, page, pageSize)
}

// DeleteComment 删除评论
// DELETE /api/feeds/:id/comments/:comment_id
func (h *FeedHandler) DeleteComment(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)

	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	commentIDStr := c.Param("comment_id")
	commentID, err := strconv.ParseUint(commentIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "评论ID无效")
		return
	}

	if err := h.feedService.DeleteComment(currentUserID, uint(feedID), uint(commentID)); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	utils.SuccessWithMessage(c, "删除评论成功", nil)
}

// GetFeedLikers 获取点赞用户列表
// GET /api/feeds/:id/likes?page=1&page_size=20
func (h *FeedHandler) GetFeedLikers(c *gin.Context) {
	feedIDStr := c.Param("id")
	feedID, err := strconv.ParseUint(feedIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "动态ID无效")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	likers, total, err := h.feedService.GetFeedLikers(uint(feedID), page, pageSize)
	if err != nil {
		utils.Error(c, 500, "获取点赞列表失败")
		return
	}

	utils.SuccessPage(c, likers, total, page, pageSize)
}
