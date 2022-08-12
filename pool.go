package nanopow

import (
	"encoding/binary"
	"runtime"
	"sync"
)

type Context struct {
	results chan uint64
	stops   chan bool
	closed  bool
	mutex   *sync.Mutex
}

func NewContext() *Context {
	return &Context{
		results: make(chan uint64, 64),
		stops:   make(chan bool),
		closed:  false,
		mutex:   new(sync.Mutex),
	}
}

func (c *Context) workerStop() <-chan bool {
	return c.stops
}

func (c *Context) workerResult(i uint64) {
	c.mutex.Lock()
	if !c.closed {
		c.results <- i
	}
	c.mutex.Unlock()
}

func (c *Context) Cancel() {
	c.mutex.Lock()
	close(c.stops)
	close(c.results)
	clear(c.results)
	c.closed = true
	c.mutex.Unlock()
}

func (c *Context) Result() (result Work) {
	r := <-c.results

	c.Cancel()

	binary.BigEndian.PutUint64(result[:], r)

	return result
}

func clear(r chan uint64) {
	for len(r) > 0 {
		<-r
	}
}

type WorkerGenerator interface {
	GenerateWork(ctx *Context, root []byte, difficulty uint64) (err error)
}

type Pool struct {
	Workers []WorkerGenerator
}

func NewPool(w ...WorkerGenerator) (p *Pool) {
	return &Pool{Workers: w}
}

func (p *Pool) GenerateWork(root []byte, difficulty uint64) (w Work, err error) {
	ctx := NewContext()

	for _, wk := range p.Workers {
		if wk == nil {
			continue
		}

		go wk.GenerateWork(ctx, root, difficulty)
	}

	return ctx.Result(), nil
}

func GenerateWork(root []byte, difficulty uint64) (w Work, err error) {
	defaultWorker := getDefaultWorkerPool()
	if defaultWorker == nil || defaultWorker.Workers == nil {
		return w, ErrNoDefaultPoolAvailable
	}

	return defaultWorker.GenerateWork(root, difficulty)
}

var DefaultWorkerPool *Pool // We don't use DefaultWorkerPool = newDefaultPool() because it slow down the init

func getDefaultWorkerPool() (p *Pool) {
	if DefaultWorkerPool == nil {
		DefaultWorkerPool = newDefaultPool()
	}

	return DefaultWorkerPool
}

func newDefaultPool() (p *Pool) {
	p = new(Pool)

	clDevice, gpuErr := GetDevice()
	if gpuErr != nil {
		gpu, gpuErr := NewWorkerGPU(clDevice)
		if gpuErr == nil {
			p.Workers = append(p.Workers, gpu)
		}
	}

	threads := runtime.NumCPU()
	if gpuErr == nil {
		if threads >= 8 {
			threads /= 2
		} else {
			return p
		}
	}

	cpu, cpuErr := NewWorkerCPUThread(uint64(threads))
	if cpuErr == nil {
		p.Workers = append(p.Workers, cpu)
	}

	return p
}
