package service

import (
	"context"

	"backend/internal/repository"
)

type AuditLogResponse struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Username   string `json:"username"`
	Action     string `json:"action"`
	EntityID   string `json:"entity_id"`
	EntityName string `json:"entity_name"`
	Details    string `json:"details"`
	CreatedAt  string `json:"created_at"`
}

type AuditService interface {
	GetAuditLogs(ctx context.Context, page, limit int) ([]AuditLogResponse, int64, error)
}

type auditService struct {
	auditRepo repository.AuditRepository
}

func NewAuditService(auditRepo repository.AuditRepository) AuditService {
	return &auditService{auditRepo: auditRepo}
}

func (s *auditService) GetAuditLogs(ctx context.Context, page, limit int) ([]AuditLogResponse, int64, error) {
	logs, total, err := s.auditRepo.List(ctx, page, limit)
	if err != nil {
		return nil, 0, err
	}

	res := make([]AuditLogResponse, 0, len(logs))
	for _, l := range logs {
		username := "System"
		userID := ""
		if l.User != nil {
			username = l.User.Username
		}
		if l.UserID != nil {
			userID = l.UserID.String()
		}

		res = append(res, AuditLogResponse{
			ID:         l.ID.String(),
			UserID:     userID,
			Username:   username,
			Action:     l.Action,
			EntityID:   l.EntityID,
			EntityName: l.EntityName,
			Details:    l.Details,
			CreatedAt:  l.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return res, total, nil
}
