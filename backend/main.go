// Package main 为应用启动入口。
// 启动顺序：配置 -> 数据库 -> Redis -> RabbitMQ -> 路由服务。
package main

import (
	"feed/cache"
	"feed/config"
	"feed/models"
	"feed/mq"
	"feed/router"
	"fmt"
	"log"
)

func main() {
	// 1. 初始化配置
	if err := config.InitConfig(); err != nil {
		log.Fatalf("Init config failed: %v", err)
	}
	log.Println("✅ Config loaded successfully")

	// 2. 初始化数据库
	if err := models.InitDB(); err != nil {
		log.Fatalf("Init database failed: %v", err)
	}
	log.Println("✅ Database initialized successfully")

	// 3. 初始化Redis
	if err := cache.InitRedis(); err != nil {
		log.Fatalf("Init redis failed: %v", err)
	}
	log.Println("✅ Redis initialized successfully")

	// 4. 初始化消息队列
	if err := mq.InitMQ(); err != nil {
		log.Fatalf("Init rabbitmq failed: %v", err)
	}
	log.Println("✅ Message queue initialized successfully")

	// 5. 设置路由并启动服务器
	r := router.SetupRouter()
	port := config.AppConfig.Server.Port
	addr := fmt.Sprintf(":%d", port)

	log.Printf("🚀 Feed System Server starting on %s", addr)
	log.Printf("📋 API Documentation: http://localhost:%d/api", port)
	log.Printf("📊 推拉混合策略:")
	log.Printf("   - 大V阈值: %d 粉丝", config.AppConfig.Feed.BigVThreshold)
	log.Printf("   - 推模式上限: %d 粉丝", config.AppConfig.Feed.PushFanLimit)
	log.Printf("   - 收件箱大小: %d", config.AppConfig.Feed.InboxMaxSize)
	log.Printf("   - 发件箱大小: %d", config.AppConfig.Feed.OutboxMaxSize)

	if err := r.Run(addr); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}
}
