package models

import (
	"feed/config"
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 为全局数据库连接句柄（进程级单例）。
var DB *gorm.DB

func InitDB() error {
	cfg := config.AppConfig.Database

	var logLevel logger.LogLevel
	if config.AppConfig.Server.Mode == "debug" {
		logLevel = logger.Info
	} else {
		logLevel = logger.Silent
	}

	dsn := cfg.DSN()
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("connect to database failed: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB failed: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)

	// 自动迁移
	err = DB.AutoMigrate(
		&User{},
		&Follow{},
		&Feed{},
		&Timeline{},
		&Like{},
		&Comment{},
		&Visit{},
		&Message{},
		&Notification{},
	)
	if err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}

	log.Println("Database initialized successfully")
	return nil
}
