package postgres_test

import (
	"github.com/google/uuid"
)

// Дрібні хелпери без БД (щоб файл компілювався незалежно).

func uuidNil() uuid.UUID { return uuid.Nil }

func newUUID() uuid.UUID { return uuid.New() }
