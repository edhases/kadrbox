package http_test

// Compile-time proof that the concrete repositories satisfy the narrow
// interfaces the handlers now depend on.
//
// This lives in the test package on purpose: the handler package must not
// import a concrete repository, so the wiring is proven here instead. If a
// repository signature drifts, `go build ./...` of the tests fails immediately
// rather than surfacing as a runtime nil dereference in production.

import (
	"github.com/edhases/oxide-server/internal/repository/postgres"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

var (
	_ transporthttp.UserStore      = (*postgres.UserRepository)(nil)
	_ transporthttp.RefreshStore   = (*redisRepo.RedisClient)(nil)
	_ transporthttp.FavoritesStore = (*postgres.FavoritesRepository)(nil)
	_ transporthttp.HistoryStore   = (*postgres.HistoryRepository)(nil)
	_ transporthttp.ContentCache   = (*postgres.CacheRepository)(nil)
)
