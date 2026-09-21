// Package grpcapi implements the generated GrpcTaskService business logic
// on an in-memory store and wires it to both transports with one shared
// policy set.
package grpcapi

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zenta-dev/zever/apperror"
	"github.com/zenta-dev/zever/authz"
	genapp "github.com/zenta-dev/zever/examples/todo/generated/gogen/app"
)

// Service is a goroutine-safe in-memory GrpcTaskService implementation.
type Service struct {
	mu    sync.Mutex
	tasks map[string]*genapp.GrpcTask
}

// New returns an empty Service.
func New() *Service {
	return &Service{tasks: make(map[string]*genapp.GrpcTask)}
}

// subject returns the caller identity stored by authz middleware/interceptor.
func subject(ctx context.Context) (string, error) {
	claims, ok := authz.ClaimsFromContext(ctx)
	if !ok || claims.Subject == "" {
		return "", apperror.New(apperror.Unauthenticated, "missing credentials")
	}
	return claims.Subject, nil
}

// CreateTask stores a task owned by the caller.
func (s *Service) CreateTask(ctx context.Context, req *genapp.CreateTaskRequest) (*genapp.GrpcTask, error) {
	sub, err := subject(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || strings.TrimSpace(req.Title) == "" {
		return nil, apperror.New(apperror.InvalidArgument, "title required")
	}

	task := &genapp.GrpcTask{
		Id:        uuid.NewString(),
		UserId:    sub,
		Title:     req.Title,
		Body:      req.Body,
		CreatedAt: timestamppb.Now(),
	}

	s.mu.Lock()
	s.tasks[task.Id] = task
	s.mu.Unlock()

	return task, nil
}

// GetTask returns one owned task; missing or foreign ids are NotFound.
func (s *Service) GetTask(ctx context.Context, req *genapp.GetTaskRequest) (*genapp.GrpcTask, error) {
	sub, err := subject(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id == "" {
		return nil, apperror.New(apperror.InvalidArgument, "id required")
	}

	s.mu.Lock()
	task, ok := s.tasks[req.Id]
	s.mu.Unlock()

	if !ok || task.UserId != sub {
		return nil, apperror.New(apperror.NotFound, "not found")
	}
	return task, nil
}

// DeleteTask removes one owned task; missing or foreign ids are NotFound.
//
// The generated DeleteTask policy carries a permission check that the
// shared rbac checker denies, so through the wired transports this method
// is unreachable without permission; the owner check here is defense in
// depth for direct calls.
func (s *Service) DeleteTask(ctx context.Context, req *genapp.DeleteTaskRequest) (*genapp.GrpcTask, error) {
	sub, err := subject(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id == "" {
		return nil, apperror.New(apperror.InvalidArgument, "id required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[req.Id]
	if !ok || task.UserId != sub {
		return nil, apperror.New(apperror.NotFound, "not found")
	}
	delete(s.tasks, req.Id)
	return task, nil
}
