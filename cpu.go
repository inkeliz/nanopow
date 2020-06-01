package nanopow

import (
	"encoding/binary"
	"golang.org/x/crypto/blake2b"
	"math"
	"runtime"
)

type cpuWorker struct {
	thread uint64
}

func NewWorkerCPU() (*cpuWorker, error) {
	return NewWorkerCPUThread(uint64(runtime.NumCPU()))
}

func NewWorkerCPUThread(threads uint64) (*cpuWorker, error) {
	return &cpuWorker{thread: threads}, nil
}

func (w *cpuWorker) GenerateWork(ctx *Context, root []byte, difficulty uint64) (err error) {
	for i := uint64(0); i < w.thread; i++ {
		go w.generateWork(ctx, root, difficulty, math.MaxUint32 + ((math.MaxUint32 / w.thread) * i))
	}

	return nil
}

func (w *cpuWorker) generateWork(ctx *Context, root []byte, difficulty uint64, result uint64) (err error) {
	h, _ := blake2b.New(8, nil)
	nonce := make([]byte, 40)
	copy(nonce[8:], root)

	for ; ; result++ {
		select {
		default:
			binary.LittleEndian.PutUint64(nonce[:8], result) // Using `binary.Write(h, ...) is worse

			h.Write(nonce)

			if binary.LittleEndian.Uint64(h.Sum(nil)) >= difficulty {
				ctx.workerResult(result)
				return nil
			}

			h.Reset()
		case <-ctx.workerStop():
			return nil
		}
	}
}
