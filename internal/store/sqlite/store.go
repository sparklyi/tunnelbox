package sqlite

import (
	"database/sql"

	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/service"
)

// Store adapts the SQLite connection to the domain repository contracts.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ServiceRepository and OperationRepository keep the two domain contracts
// separate even though they share one SQLite connection.
type ServiceRepository struct {
	db *sql.DB
}

type OperationRepository struct {
	db *sql.DB
}

func (s *Store) Services() *ServiceRepository {
	return &ServiceRepository{db: s.db}
}

func (s *Store) Operations() *OperationRepository {
	return &OperationRepository{db: s.db}
}

var _ service.Repository = (*ServiceRepository)(nil)
var _ operation.Repository = (*OperationRepository)(nil)
