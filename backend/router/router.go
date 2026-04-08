package router

import (
	"feed-system/config"
	"feed-system/handlers"
	"feed-system/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	if config.AppConfig.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// 全局中间件
	r.Use(middleware.CorsMiddleware())

	// 上传静态文件访问
	r.Static("/uploads", config.AppConfig.Upload.Path)

	// 初始化Handler
	userHandler := handlers.NewUserHandler()
	followHandler := handlers.NewFollowHandler()
	feedHandler := handlers.NewFeedHandler()
	uploadHandler := handlers.NewUploadHandler()

	// API路由组
	api := r.Group("/api")
	{
		// ==================== 认证相关（无需登录） ====================
		auth := api.Group("/auth")
		{
			auth.POST("/register", userHandler.Register)
			auth.POST("/login", userHandler.Login)
		}

		// ==================== 需要登录的路由 ====================
		authenticated := api.Group("")
		authenticated.Use(middleware.AuthMiddleware())
		{
			// 用户相关
			authenticated.GET("/users/me", userHandler.GetCurrentUser)
			authenticated.PUT("/users/me", userHandler.UpdateProfile)
			authenticated.GET("/users/me/visitors", userHandler.GetRecentVisitors)
			authenticated.GET("/users/search", userHandler.SearchUsers)
			authenticated.GET("/users/:id", userHandler.GetProfile)

			// 上传相关
			authenticated.POST("/upload/image", uploadHandler.UploadImage)
			authenticated.POST("/upload/video", uploadHandler.UploadVideo)

			// 关注相关
			authenticated.POST("/follow/:id", followHandler.Follow)
			authenticated.DELETE("/follow/:id", followHandler.Unfollow)
			authenticated.GET("/users/:id/followers", followHandler.GetFollowers)
			authenticated.GET("/users/:id/following", followHandler.GetFollowing)

			// Feed相关
			authenticated.POST("/feeds", feedHandler.PublishFeed)
			authenticated.POST("/feeds/repost", feedHandler.RepostFeed)
			authenticated.DELETE("/feeds/:id", feedHandler.DeleteFeed)
			authenticated.GET("/feeds/:id", feedHandler.GetFeed)
			authenticated.GET("/users/:id/feeds", feedHandler.GetUserFeeds)

			// 时间线
			authenticated.GET("/timeline", feedHandler.GetTimeline)

			// 点赞
			authenticated.POST("/feeds/:id/like", feedHandler.LikeFeed)
			authenticated.DELETE("/feeds/:id/like", feedHandler.UnlikeFeed)
			authenticated.GET("/feeds/:id/likes", feedHandler.GetFeedLikers)

			// 评论
			authenticated.POST("/feeds/:id/comments", feedHandler.CommentFeed)
			authenticated.GET("/feeds/:id/comments", feedHandler.GetComments)
			authenticated.DELETE("/feeds/:id/comments/:comment_id", feedHandler.DeleteComment)
		}
	}

	return r
}
