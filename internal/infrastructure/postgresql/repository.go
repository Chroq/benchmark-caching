package postgresql

import (
	"github.com/Chroq/benchmark-caching/internal/domain/port/output"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/optimized"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/standard"
	tsidrepo "github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/tsid"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OptimizedRepository = optimized.OptimizedRepository
type StandardRepository = standard.StandardRepository

// NewOptimizedRepository delegates creation to the optimized subpackage.
func NewOptimizedRepository(pool *pgxpool.Pool) output.OptimizedUserRepository {
	return optimized.NewRepository(pool)
}

// NewStandardRepository delegates creation to the standard subpackage.
func NewStandardRepository(pool *pgxpool.Pool) output.StandardUserRepository {
	return standard.NewRepository(pool)
}

// NewTSIDRepository delegates creation to the tsid subpackage.
func NewTSIDRepository(pool *pgxpool.Pool) output.UserRepository {
	return tsidrepo.NewRepository(pool)
}

// PopulateStandardTable delegates population to the standard subpackage.
var PopulateStandardTable = standard.PopulateStandardTable

// PopulateTSIDTable delegates population to the tsid subpackage.
var PopulateTSIDTable = tsidrepo.PopulateTSIDTable


