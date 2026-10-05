package http_test

// Test doubles that make failure modes reachable from the handler tests.
//
// The in-memory user store is a happy-path fake: every write succeeds. Most of
// the defects fixed here were about what the handlers do when a write fails or
// returns something unexpected, so those paths need fakes that can fail.

import (
	"context"
	"errors"

	"github.com/edhases/oxide-server/internal/domain"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/google/uuid"
)

// errStoreUnavailable stands in for a database that is reachable but failing.
var errStoreUnavailable = errors.New("store unavailable")

// failingCreateStore fails CreateUser with an error that is NOT a uniqueness
// violation, and creates no row.
//
// It models the case the old Register handler got wrong: it answered 409 "email
// already registered" for every CreateUser error, so a transient database
// failure read to the client as "this address is taken".
type failingCreateStore struct {
	*memUserStore
}

func (s *failingCreateStore) CreateUser(ctx context.Context, email, passwordHash, username string) (*domain.User, error) {
	return nil, errStoreUnavailable
}

// failingVerifyStore fails MarkEmailVerified on an existing user.
//
// It models the self-contradictory state the old handlers produced: a token was
// issued with IsVerified=true while the row still said false, and
// RequireVerifiedEmail then 403'd that user on their very next request.
type failingVerifyStore struct {
	*memUserStore
}

func (s *failingVerifyStore) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	return errStoreUnavailable
}

var (
	_ transporthttp.UserStore = (*failingCreateStore)(nil)
	_ transporthttp.UserStore = (*failingVerifyStore)(nil)
)
