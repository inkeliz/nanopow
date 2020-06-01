package nanopow

import (
	"math/rand"
	"testing"
)

func TestGenerateWork(t *testing.T) {
	rand.Seed(1) // To make always test the same

	hash, difficulty := make([]byte, 32), CalculateDifficulty(8)
	rand.Read(hash)

	w, err := GenerateWork(hash, difficulty)
	if err != nil {
		t.Error(err)
	}

	if IsValid(hash, difficulty, w) == false {
		t.Error("create invalid work")
	}

	return
}

func TestGenerateWork2(t *testing.T) {
	rand.Seed(42) // To make always test the same

	for n := 0; n < 1; n++ {
		hash, difficulty := make([]byte, 32), V1BaseDifficult
		rand.Read(hash)

		w, err := GenerateWork(hash, difficulty)
		if err != nil {
			t.Error(err)
		}

		if IsValid(hash, difficulty, w) == false {
			t.Error("create invalid work", w[:], hash)
		}
	}

	return
}
