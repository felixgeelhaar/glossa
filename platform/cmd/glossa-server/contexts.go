package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/coverage"
	catalogapi "github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/httpapi"
	catalogpg "github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/postgres"
	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	catalogport "github.com/felixgeelhaar/glossa/platform/internal/localization/adapters/catalog"
	localizationapi "github.com/felixgeelhaar/glossa/platform/internal/localization/adapters/httpapi"
	localizationpg "github.com/felixgeelhaar/glossa/platform/internal/localization/adapters/postgres"
	localizationapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
)

// contexts are the bounded contexts besides Identity, wired to each
// other only through their application ports: Localization reads the
// catalog through catalogport, Catalog asks Localization for
// translation coverage through coverage.
type contexts struct {
	catalogAPI      *catalogapi.API
	localizationAPI *localizationapi.API
}

// newContexts builds Catalog and Localization and subscribes
// Localization to Catalog's events.
func newContexts(pool *pgxpool.Pool, events *outbox.Registry) (contexts, error) {
	uow := db.NewUnitOfWork(pool)
	catalog := catalogapp.New(catalogpg.NewTransactor(uow))
	localization := localizationapp.New(localizationpg.NewTransactor(uow), catalogport.New(catalog))
	catalog.SetCoverage(coverage.New(localization))
	if err := localization.Subscribe(events); err != nil {
		return contexts{}, err
	}
	return contexts{catalogAPI: catalogapi.New(catalog), localizationAPI: localizationapi.New(localization)}, nil
}
