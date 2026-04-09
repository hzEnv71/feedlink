package repository

import (
	"feed/models"

	"gorm.io/gorm"
)

// MessageRepository 定义私信域数据访问契约。
type MessageRepository interface {
	Create(message *models.Message) error
	GetByConversation(currentUserID, targetUserID uint, page, pageSize int) ([]models.Message, int64, error)
	ListByUser(userID uint) ([]models.Message, error)
	CountUnreadFromTo(fromUserID, toUserID uint) (int64, error)
	MarkReadFromTo(fromUserID, toUserID uint) error
}

type messageMySQLRepository struct {
	db *gorm.DB
}

func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageMySQLRepository{db: db}
}

func (r *messageMySQLRepository) Create(message *models.Message) error {
	return r.db.Create(message).Error
}

func (r *messageMySQLRepository) GetByConversation(currentUserID, targetUserID uint, page, pageSize int) ([]models.Message, int64, error) {
	var messages []models.Message
	var total int64

	query := r.db.Model(&models.Message{}).
		Where("(from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?)",
			currentUserID, targetUserID, targetUserID, currentUserID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Preload("FromUser").Preload("ToUser").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&messages).Error; err != nil {
		return nil, 0, err
	}

	return messages, total, nil
}

func (r *messageMySQLRepository) ListByUser(userID uint) ([]models.Message, error) {
	var messages []models.Message
	if err := r.db.Where("from_user_id = ? OR to_user_id = ?", userID, userID).
		Order("created_at DESC").
		Preload("FromUser").
		Preload("ToUser").
		Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

func (r *messageMySQLRepository) CountUnreadFromTo(fromUserID, toUserID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Message{}).
		Where("from_user_id = ? AND to_user_id = ? AND is_read = ?", fromUserID, toUserID, false).
		Count(&count).Error
	return count, err
}

func (r *messageMySQLRepository) MarkReadFromTo(fromUserID, toUserID uint) error {
	return r.db.Model(&models.Message{}).
		Where("from_user_id = ? AND to_user_id = ? AND is_read = ?", fromUserID, toUserID, false).
		Update("is_read", true).Error
}
