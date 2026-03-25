package repository

import (
	"context"
	"pai-smart-go/internal/model"

	"gorm.io/gorm"
)

type ConversationTurnRepository interface {
	Create(ctx context.Context, turn *model.ConversationTurn) error
	Update(ctx context.Context, turn *model.ConversationTurn) error
	GetByID(ctx context.Context, turnID string) (*model.ConversationTurn, error)
	GetByRequestID(ctx context.Context, requestID string) (*model.ConversationTurn, error)
}

type conversationTurnRepository struct {
	db *gorm.DB
}

func NewConversationTurnRepository(db *gorm.DB) ConversationTurnRepository {
	return &conversationTurnRepository{db: db}
}

func (r *conversationTurnRepository) Create(ctx context.Context, turn *model.ConversationTurn) error {
	return r.db.WithContext(ctx).Create(turn).Error
}

func (r *conversationTurnRepository) Update(ctx context.Context, turn *model.ConversationTurn) error {
	return r.db.WithContext(ctx).Save(turn).Error
}

func (r *conversationTurnRepository) GetByID(ctx context.Context, turnID string) (*model.ConversationTurn, error) {
	var turn model.ConversationTurn
	if err := r.db.WithContext(ctx).Where("id = ?", turnID).First(&turn).Error; err != nil {
		return nil, err
	}
	return &turn, nil
}

func (r *conversationTurnRepository) GetByRequestID(ctx context.Context, requestID string) (*model.ConversationTurn, error) {
	var turn model.ConversationTurn
	if err := r.db.WithContext(ctx).Where("request_id = ?", requestID).First(&turn).Error; err != nil {
		return nil, err
	}
	return &turn, nil
}
