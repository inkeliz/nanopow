// +build vk

package nanopow

type vkWorker struct {}

func NewWorkerGPU() (*vkWorker, error) {
	return NewWorkerGPUThread(0)
}

func NewWorkerGPUThread(_ uint64) (*vkWorker, error) {
	return nil, ErrNotSupported
}

func (w *vk) GenerateWork(ctx *Context, root []byte, difficulty uint64) (err error) {
	return ErrNotSupported
}