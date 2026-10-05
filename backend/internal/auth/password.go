package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidHash         = errors.New("the encoded hash is not in the correct format")
	ErrIncompatibleVersion = errors.New("incompatible version of argon2")
	// ErrKDFBusy means too many Argon2 derivations are already in flight. It is
	// not a client mistake: the process is protecting itself from OOM, so the
	// caller should answer 503 and let the client retry.
	ErrKDFBusy = errors.New("password hashing capacity exhausted, retry shortly")
)

type Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultParams is the OWASP-recommended floor for Argon2id (m=64MiB, t=3,
// p=2). It is deliberately NOT weakened to bcrypt or to a cheaper memory cost:
// the deployment runs in a 256 MiB container, and the answer to "the KDF is
// expensive here" is to bound concurrency (kdfSem), never to make the stored
// hash cheaper for an offline attacker who already holds the database.
var DefaultParams = &Params{
	Memory:      64 * 1024, // 64 MB
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// defaultKDFConcurrency is sized for a 256 MiB container: two concurrent 64 MiB
// derivations plus the Go heap fit; a dozen do not.
const defaultKDFConcurrency = 2

var (
	kdfSemOnce sync.Once
	kdfSem     chan struct{}
)

func initKDFSem() {
	n := defaultKDFConcurrency
	if v := strings.TrimSpace(os.Getenv("ARGON2_MAX_CONCURRENCY")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}
	kdfSem = make(chan struct{}, n)
}

// KDFConcurrency reports how many Argon2 derivations may run at once.
func KDFConcurrency() int {
	kdfSemOnce.Do(initKDFSem)
	return cap(kdfSem)
}

// acquireKDFSlot reserves one derivation slot without blocking.
//
// Queueing instead of rejecting would be friendlier, but a queue of pending
// Argon2 requests is just as good a memory-exhaustion primitive as unbounded
// concurrency: requests wait in RAM holding their bodies, and every waiter
// eventually runs. Failing fast turns an OOM into a retryable 503.
func acquireKDFSlot() error {
	kdfSemOnce.Do(initKDFSem)
	select {
	case kdfSem <- struct{}{}:
		return nil
	default:
		return ErrKDFBusy
	}
}

func releaseKDFSlot() {
	<-kdfSem
}

// HashPassword хешує пароль за стандартом Argon2id
func HashPassword(password string) (string, error) {
	if err := acquireKDFSlot(); err != nil {
		return "", err
	}
	defer releaseKDFSlot()

	return hashPasswordUngated(password)
}

// hashPasswordUngated performs the derivation with no concurrency gate. It is
// used by HashPassword (which gates first) and to build the process-wide dummy
// hash, which must not queue behind the gate it will be measured against.
func hashPasswordUngated(password string) (string, error) {
	salt := make([]byte, DefaultParams.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		DefaultParams.Iterations,
		DefaultParams.Memory,
		DefaultParams.Parallelism,
		DefaultParams.KeyLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		DefaultParams.Memory,
		DefaultParams.Iterations,
		DefaultParams.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// ComparePasswordAndHash перевіряє пароль з Argon2id хешем
//
// The derivation is gated by the same semaphore as HashPassword: on the login
// path this is the only KDF work and it costs exactly as much memory.
func ComparePasswordAndHash(password, encodedHash string) (bool, error) {
	vals := strings.Split(encodedHash, "$")
	if len(vals) != 6 {
		return false, ErrInvalidHash
	}

	if vals[1] != "argon2id" {
		return false, ErrIncompatibleVersion
	}

	var version int
	if _, err := fmt.Sscanf(vals[2], "v=%d", &version); err != nil {
		return false, err
	}
	if version != argon2.Version {
		return false, ErrIncompatibleVersion
	}

	params := &Params{}
	if _, err := fmt.Sscanf(vals[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(vals[4])
	if err != nil {
		return false, err
	}

	hash, err := base64.RawStdEncoding.DecodeString(vals[5])
	if err != nil {
		return false, err
	}
	params.KeyLength = uint32(len(hash))

	// A stored hash can predate a parameter change, but the parameters are read
	// from a database row, so cap what an oversized m=/t= value can make us
	// allocate.
	if params.Memory > DefaultParams.Memory*4 || params.Iterations > 32 {
		return false, ErrIncompatibleVersion
	}

	if err := acquireKDFSlot(); err != nil {
		return false, err
	}
	defer releaseKDFSlot()

	comparisonHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	if subtle.ConstantTimeCompare(hash, comparisonHash) == 1 {
		return true, nil
	}

	return false, nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// dummyHashValue returns a valid Argon2id hash string, created on first use from
// a random password nobody knows, with the same parameters as a real hash. That
// makes the derivation below cost the same wall-clock time as a genuine check.
func dummyHashValue() string {
	dummyHashOnce.Do(func() {
		pw := make([]byte, 32)
		if _, err := rand.Read(pw); err != nil {
			pw = []byte("oxide-dummy-fallback-password")
		}
		encoded, err := hashPasswordUngated(string(pw))
		if err != nil {
			// Unreachable in practice (rand.Read just failed above too), but a
			// structurally valid hash keeps the caller's timing unchanged.
			encoded = "$argon2id$v=19$m=65536,t=3,p=2$" +
				base64.RawStdEncoding.EncodeToString(make([]byte, 16)) + "$" +
				base64.RawStdEncoding.EncodeToString(make([]byte, 32))
		}
		dummyHash = encoded
	})
	return dummyHash
}

// BurnKDFForDummyUser runs one Argon2id derivation against the dummy hash.
//
// Without it, Login answers "unknown account" having done no KDF work at all,
// which makes response time a reliable account-existence oracle. Errors are
// swallowed on purpose: the caller is about to answer 401 either way, and
// ErrKDFBusy here is not something the client can act on.
func BurnKDFForDummyUser(password string) {
	_, _ = ComparePasswordAndHash(password, dummyHashValue())
}
