// +build !cl,!vk,!gl

package nanopow

type noneGPUWorker struct {}

func NewWorkerGPU() (*noneGPUWorker, error) {
	return NewWorkerGPUThread(0)
}

func NewWorkerGPUThread(_ uint64) (*noneGPUWorker, error) {
	return nil, ErrNotSupported
}

func (w *noneGPUWorker) GenerateWork(ctx *Context, root []byte, difficulty uint64) (err error) {
	return ErrNotSupported
}