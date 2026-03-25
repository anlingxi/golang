package repository

import (
	"context"
	"pai-smart-go/internal/model"

	"gorm.io/gorm"
)

type SessionRepository interface {
	Create(ctx context.Context, session *model.Session) error
	GetByID(ctx context.Context, sessionID string) (*model.Session, error)
	Update(ctx context.Context, session *model.Session) error
	UpdateLastMessageAt(ctx context.Context, sessionID string) error
}

type sessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) SessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) Create(ctx context.Context, session *model.Session) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *sessionRepository) GetByID(ctx context.Context, sessionID string) (*model.Session, error) {
	var session model.Session
	if err := r.db.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *sessionRepository) Update(ctx context.Context, session *model.Session) error {
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *sessionRepository) UpdateLastMessageAt(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Model(&model.Session{}).
		Where("id = ?", sessionID).
		Update("last_message_at", gorm.Expr("NOW()")).
		Error
}
