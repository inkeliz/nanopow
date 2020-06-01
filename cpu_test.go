package nanopow

import (
	"math/rand"
	"testing"
)

func TestNewWorkerCPU(t *testing.T) {
	rand.Seed(1) // To make always test the same

	hash, difficulty := make([]byte, 32), CalculateDifficulty(0)
	rand.Read(hash)

	cpu, err := NewWorkerCPU()
	if err != nil {
		t.Fatal(err)
	}

	ctx := NewContext()

	if err := cpu.GenerateWork(ctx, hash, difficulty); err != nil {
		t.Error(err)
	}

	w := ctx.Result()

	if IsValid(hash, difficulty, w) == false {
		t.Error("create invalid work")
	}

	return
}