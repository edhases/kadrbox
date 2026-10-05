package postgres

import (
	"errors"
	"testing"
)

// TestCovIsDuplicateKeyErrorTable — ХАРАКТЕРИЗАЦІЯ (без БД, isDuplicateKeyError чиста).
// NOTE: опис задачі припускав, що функція шукає "23505" лише в перших 5 символах
// і реальний формат pgx дає FALSE (баг). Фактична реалізація (user_repo.go)
// шукає підрядок по ВСЬОМУ повідомленню через containsStr, тому реальний формат
// pgx "... (SQLSTATE 23505)" дає TRUE. Тест зафіксовано під фактичну поведінку.
func TestCovIsDuplicateKeyErrorTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"emptyError", errors.New(""), false},
		{"nilError", nil, false},
		{
			"realPgxFormat",
			errors.New(`ERROR: duplicate key value violates unique constraint "users_email_key" (SQLSTATE 23505)`),
			true,
		},
		{"syntheticCodePrefix", errors.New("23505: duplicate key"), true},
		{
			"constraintTextOnly",
			errors.New(`duplicate key value violates unique constraint "users_email_key"`),
			true,
		},
		{"unrelatedError", errors.New("connection refused"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDuplicateKeyError(tc.err); got != tc.want {
				t.Errorf("isDuplicateKeyError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
