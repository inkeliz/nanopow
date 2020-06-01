package nanopow

import (
	"encoding/binary"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/exp/errors"
)

var (
	ErrNotSupported           = errors.New("that device don't support that type of work method")
	ErrNoDeviceAvailable      = errors.New("no device found")
	ErrNoDefaultPoolAvailable = errors.New("no default pool found")
)

var (
	V1BaseDifficult    = CalculateDifficulty(0)
	V2BaseDifficult    = CalculateDifficulty(8)
	V2ReceiveDifficult = CalculateDifficulty(-8)
)

const (
	baseMaxUint64  = uint64(1<<64 - 1)
	baseDifficulty = baseMaxUint64 - uint64(0xffffffc000000000)
)

func CalculateDifficulty(multiplier int64) uint64 {
	if multiplier < 0 {
		return baseMaxUint64 - (baseDifficulty * ((baseMaxUint64 - uint64(multiplier)) + 1))
	}

	if multiplier == 0 {
		multiplier = 1
	}

	return baseMaxUint64 - (baseDifficulty / uint64(multiplier))
}

type Work [8]byte

func NewWork(b []byte) (work Work) {
	copy(work[:], b)
	return work
}

func IsValid(previous []byte, difficult uint64, w Work) bool {
	n := make([]byte, 8)
	copy(n, w[:])

	reverse(n)

	return isValid(previous, difficult, n)
}

func isValid(previous []byte, difficult uint64, w []byte) bool {
	hash, err := blake2b.New(8, nil)
	if err != nil {
		return false
	}

	hash.Write(w)
	hash.Write(previous[:])

	return binary.LittleEndian.Uint64(hash.Sum(nil)) >= difficult
}


func reverse(v []byte) {
	// binary.LittleEndian.PutUint64(v, binary.BigEndian.Uint64(v))
	v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7] = v[7], v[6], v[5], v[4], v[3], v[2], v[1], v[0] // It's works. LOL
}
