// Package usecase implements application business logic. Each logic group in own file.
package usecase

import (
	"id-maker/internal/entity"
)

//go:generate mockgen -source=interfaces.go -destination=./mocks_test.go -package=usecase_test

type (
	Segment interface {
		CreateTag(*entity.Segments) error
		GetId(string) (int64, error)
		SnowFlakeGetId() int64
		GetBatchId(string, int64) ([]int64, error)
	}

	SegmentRepo interface {
		GetList() ([]entity.Segments, error)
		GetNextId(string) (*entity.Segments, error)
		Add(*entity.Segments) error
		GetStep(string) (int64, error)
	}
)
