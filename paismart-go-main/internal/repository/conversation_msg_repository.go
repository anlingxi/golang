package repository

import (
	"context"
	"pai-smart-go/internal/model"

	"gorm.io/gorm"
)

type ConversationMsgRepository interface {
	Create(ctx context.Context, msg *model.ConversationMsg) error
	BatchCreate(ctx context.Context, msgs []*model.ConversationMsg) error
	ListRecentBySessionID(ctx context.Context, sessionID string, limit int) ([]model.ConversationMsg, error)
	GetMaxSequence(ctx context.Context, sessionID string) (int64, error)
}

type conversationMsgRepository struct {
	db *gorm.DB
}

func NewConversationMsgRepository(db *gorm.DB) ConversationMsgRepository {
	return &conversationMsgRepository{db: db}
}

func (r *conversationMsgRepository) Create(ctx context.Context, msg *model.ConversationMsg) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *conversationMsgRepository) BatchCreate(ctx context.Context, msgs []*model.ConversationMsg) error {
	return r.db.WithContext(ctx).Create(&msgs).Error
}

// ListRecentBySessionID 根据 sessionID 列出最近的消息，按照 sequence 正序排列。
func (r *conversationMsgRepository) ListRecentBySessionID(ctx context.Context, sessionID string, limit int) ([]model.ConversationMsg, error) {
	var msgs []model.ConversationMsg
	if limit <= 0 {
		limit = 20
	}

	// 先倒序取最近 N 条
	// 为什么要用带ctx的db？因为在一些场景下，我们可能需要设置超时或者取消查询操作，使用带ctx的db可以更好地控制查询的生命周期。
	// 为了防止sql注入，要使用参数化查询，不能直接拼接字符串。gorm的where方法会自动处理参数化查询，所以我们只需要传入sessionID作为参数即可。
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("sequence DESC").
		Limit(limit).
		Find(&msgs).Error; err != nil {
		return nil, err
	}

	// 再原地反转成正序
	// 不可以直接用slices.Reverse吗？因为slices.Reverse是Go 1.21引入的，如果我们需要兼容更早版本的Go，就不能使用这个函数了，
	// 所以只能手动反转了。
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	return msgs, nil
}

func (r *conversationMsgRepository) GetMaxSequence(ctx context.Context, sessionID string) (int64, error) {
	var maxSeq int64
	row := r.db.WithContext(ctx).
		Model(&model.ConversationMsg{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(sequence), 0)").
		Row()

	if err := row.Scan(&maxSeq); err != nil {
		return 0, err
	}
	return maxSeq, nil
}
