package flatservice

import (
	"context"
	"errors"
	"net/http"

	"go-server/internal/models"
	"go-server/internal/repository"
)

// ==================== INTERFACE ====================

type FlatService interface {
	// CreateFlat adds a new flat under a society.
	// Fails with 409 Conflict if the flat_number already exists in that society.
	CreateFlat(ctx context.Context, req *models.CreateFlatRequest) (*models.Flat, error)

	// GetByID fetches a single flat by primary key.
	GetByID(ctx context.Context, flatID int64) (*models.Flat, error)

	// GetByNumber fetches a flat using society ID + flat number.
	GetByNumber(ctx context.Context, societyID int64, flatNumber string) (*models.Flat, error)

	// ListBySociety returns all flats for a society ordered by flat number.
	ListBySociety(ctx context.Context, societyID int64) ([]*models.Flat, error)

	// UpdateFlat applies partial updates (flat_number, floor, block) to an existing flat.
	UpdateFlat(ctx context.Context, flatID int64, req *models.UpdateFlatRequest) (*models.Flat, error)

	// ActivateFlat marks a flat as active.
	ActivateFlat(ctx context.Context, flatID int64) (*models.Flat, error)

	// DeactivateFlat marks a flat as inactive.
	DeactivateFlat(ctx context.Context, flatID int64) (*models.Flat, error)

	// DeleteFlat permanently removes a flat.
	DeleteFlat(ctx context.Context, flatID int64) error

	// ValidateFlatBelongsToSociety asserts flatID is owned by societyID.
	// Use before any cross-entity operation to prevent data leaks.
	ValidateFlatBelongsToSociety(ctx context.Context, flatID int64, societyID int64) error
}

// ==================== IMPLEMENTATION ====================

type flatService struct {
	flatRepo repository.FlatRepository
}

func NewFlatService(flatRepo repository.FlatRepository) FlatService {
	return &flatService{flatRepo: flatRepo}
}

// ==================== ERROR HELPERS ====================

func notFoundErr() *models.AppError {
	return models.NewAppError(models.ErrCodeNotFound, "flat not found", http.StatusNotFound, nil)
}

func conflictErr() *models.AppError {
	return models.NewAppError(models.ErrCodeConflict, "flat number already exists in this society", http.StatusConflict, nil)
}

func internalErr(err error) *models.AppError {
	return models.NewAppError(models.ErrCodeInternalServer, "an unexpected error occurred", http.StatusInternalServerError, err)
}

// mapRepoErr converts sentinel repository errors into domain AppErrors.
func mapRepoErr(err error) error {
	if errors.Is(err, repository.ErrFlatNotFound) {
		return notFoundErr()
	}
	if errors.Is(err, repository.ErrFlatAlreadyExists) {
		return conflictErr()
	}
	return internalErr(err)
}

// ==================== METHODS ====================

func (s *flatService) CreateFlat(ctx context.Context, req *models.CreateFlatRequest) (*models.Flat, error) {
	existing, err := s.flatRepo.GetFlatByNumber(ctx, req.SocietyID, req.FlatNumber)
	if err != nil && !errors.Is(err, repository.ErrFlatNotFound) {
		return nil, internalErr(err)
	}
	if existing != nil {
		return nil, conflictErr()
	}

	flat := &models.Flat{
		SocietyID:  req.SocietyID,
		FlatNumber: req.FlatNumber,
		Block:      req.Block,
		Floor:      req.Floor,
		Status:     models.FlatStatusActive,
	}

	created, err := s.flatRepo.CreateFlat(ctx, flat)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return created, nil
}

func (s *flatService) GetByID(ctx context.Context, flatID int64) (*models.Flat, error) {
	flat, err := s.flatRepo.GetFlatByID(ctx, flatID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return flat, nil
}

func (s *flatService) GetByNumber(ctx context.Context, societyID int64, flatNumber string) (*models.Flat, error) {
	flat, err := s.flatRepo.GetFlatByNumber(ctx, societyID, flatNumber)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return flat, nil
}

func (s *flatService) ListBySociety(ctx context.Context, societyID int64) ([]*models.Flat, error) {
	flats, err := s.flatRepo.GetFlatsBySocietyID(ctx, societyID)
	if err != nil {
		return nil, internalErr(err)
	}
	return flats, nil
}

func (s *flatService) UpdateFlat(ctx context.Context, flatID int64, req *models.UpdateFlatRequest) (*models.Flat, error) {
	flat, err := s.flatRepo.GetFlatByID(ctx, flatID)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	if req.FlatNumber != nil && *req.FlatNumber != flat.FlatNumber {
		existing, checkErr := s.flatRepo.GetFlatByNumber(ctx, flat.SocietyID, *req.FlatNumber)
		if checkErr != nil && !errors.Is(checkErr, repository.ErrFlatNotFound) {
			return nil, internalErr(checkErr)
		}
		if existing != nil {
			return nil, conflictErr()
		}
		flat.FlatNumber = *req.FlatNumber
	}
	if req.Floor != nil {
		flat.Floor = req.Floor
	}
	if req.Block != nil {
		flat.Block = req.Block
	}

	updated, err := s.flatRepo.UpdateFlat(ctx, flat)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return updated, nil
}

func (s *flatService) ActivateFlat(ctx context.Context, flatID int64) (*models.Flat, error) {
	if _, err := s.flatRepo.GetFlatByID(ctx, flatID); err != nil {
		return nil, mapRepoErr(err)
	}
	updated, err := s.flatRepo.UpdateFlatStatus(ctx, flatID, models.FlatStatusActive)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return updated, nil
}

func (s *flatService) DeactivateFlat(ctx context.Context, flatID int64) (*models.Flat, error) {
	if _, err := s.flatRepo.GetFlatByID(ctx, flatID); err != nil {
		return nil, mapRepoErr(err)
	}
	updated, err := s.flatRepo.UpdateFlatStatus(ctx, flatID, models.FlatStatusInactive)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return updated, nil
}

func (s *flatService) DeleteFlat(ctx context.Context, flatID int64) error {
	if err := s.flatRepo.DeleteFlat(ctx, flatID); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

func (s *flatService) ValidateFlatBelongsToSociety(ctx context.Context, flatID int64, societyID int64) error {
	flat, err := s.flatRepo.GetFlatByID(ctx, flatID)
	if err != nil {
		return mapRepoErr(err)
	}
	if flat.SocietyID != societyID {
		return models.NewAppError(
			models.ErrCodeForbidden,
			"flat does not belong to this society",
			http.StatusForbidden,
			nil,
		)
	}
	return nil
}
