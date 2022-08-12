//go:build cl
// +build cl

package nanopow

import (
	"github.com/Inkeliz/go-opencl/opencl"
	"unsafe"
)

type clBuffer struct {
	Buffer opencl.Buffer
	size   uint64
	index  uint32
}

type clWorker struct {
	thread           uint64
	attempt          uint64
	context          opencl.Context
	device           opencl.Device
	queue            opencl.CommandQueue
	program          opencl.Program
	kernel           opencl.Kernel
	AttemptBuffer    clBuffer
	ResultBuffer     clBuffer
	ItemBuffer       clBuffer
	DifficultyBuffer clBuffer
	ResultHashBuffer clBuffer
}

func NewWorkerGPU(device opencl.Device) (*clWorker, error) {
	return NewWorkerGPUThread(1 << 23)
}

func NewWorkerGPUThread(thread uint64, device opencl.Device) (*clWorker, error) {
	if err != nil {
		return nil, err
	}

	c := &clWorker{
		thread:           thread,
		device:           device,
		AttemptBuffer:    clBuffer{size: 8},
		ResultBuffer:     clBuffer{size: 8},
		ItemBuffer:       clBuffer{size: 32},
		DifficultyBuffer: clBuffer{size: 8},
		ResultHashBuffer: clBuffer{size: 8},
	}

	err = c.init()
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (w *clWorker) GenerateWork(ctx *Context, root []byte, difficulty uint64) (err error) {
	if err = w.newQueue(); err != nil {
		return err
	}

	defer w.queue.Release()

	attempt, result, resulthash := uint64(0), uint64(0), make([]byte, 8)
	for ; ; attempt += w.thread {
		select {
		default:
			if err = w.queue.EnqueueWriteBuffer(w.AttemptBuffer.Buffer, false, w.AttemptBuffer.size, unsafe.Pointer(&attempt)); err != nil {
				return err
			}

			if err = w.queue.EnqueueWriteBuffer(w.ItemBuffer.Buffer, false, w.ItemBuffer.size, unsafe.Pointer(&root[0])); err != nil {
				return err
			}

			if err = w.queue.EnqueueWriteBuffer(w.DifficultyBuffer.Buffer, false, w.DifficultyBuffer.size, unsafe.Pointer(&difficulty)); err != nil {
				return err
			}

			if err = w.queue.EnqueueWriteBuffer(w.ResultBuffer.Buffer, false, w.ResultBuffer.size, unsafe.Pointer(&result)); err != nil {
				return err
			}

			if err = w.queue.EnqueueWriteBuffer(w.ResultHashBuffer.Buffer, false, w.ResultHashBuffer.size, unsafe.Pointer(&resulthash[0])); err != nil {
				return err
			}

			if err = w.queue.EnqueueNDRangeKernel(w.kernel, 1, []uint64{w.thread}); err != nil {
				return err
			}

			if err = w.queue.EnqueueReadBuffer(w.ResultBuffer.Buffer, false, w.ResultBuffer.size, unsafe.Pointer(&result)); err != nil {
				return err
			}

			if err = w.queue.EnqueueReadBuffer(w.ResultHashBuffer.Buffer, false, w.ResultHashBuffer.size, unsafe.Pointer(&resulthash[0])); err != nil {
				return err
			}

			w.queue.Finish()

			if result != 0 {
				ctx.workerResult(result)
				return nil
			}
		case <-ctx.workerStop():
			return nil
		}
	}
}
func (w *clWorker) newQueue() (err error) {
	w.queue, err = w.context.CreateCommandQueue(w.device)
	if err != nil {
		return err
	}

	for i, buf := range []*clBuffer{&w.AttemptBuffer, &w.ResultBuffer, &w.ItemBuffer, &w.DifficultyBuffer, &w.ResultHashBuffer} {
		if buf.Buffer, err = w.context.CreateBuffer(nil, buf.size); err != nil {
			return err
		}

		if err = w.kernel.SetArg(uint32(i), buf.Buffer.Size(), &buf.Buffer); err != nil {
			return err
		}
	}

	return nil
}

func (w *clWorker) init() (err error) {
	w.context, err = w.device.CreateContext()
	if err != nil {
		return err
	}

	w.program, err = w.context.CreateProgramWithSource(programOpenCL)
	if err != nil {
		return err
	}

	err = w.program.Build(w.device, nil)
	if err != nil {
		return err
	}

	w.kernel, err = w.program.CreateKernel("nano_work")
	if err != nil {
		return err
	}

	return nil
}

func GetDevice() (dv opencl.Device, err error) {
	platforms, err := opencl.GetPlatforms()
	if err != nil {
		return dv, ErrNoDeviceAvailable
	}

	for _, p := range platforms {
		devices, err := p.GetDevices(opencl.DeviceTypeGPU)
		if err != nil {
			return dv, ErrNoDeviceAvailable
		}

		for _, d := range devices {
			return d, nil
		}
	}

	return dv, ErrNoDeviceAvailable
}

// @TODO (inkeliz) Optimize it to fixed size (input: 40 byte | output: 8)
var programOpenCL = `
enum Blake2b_IV
{
	iv0 = 0x6a09e667f3bcc908UL,
	iv1 = 0xbb67ae8584caa73bUL,
	iv2 = 0x3c6ef372fe94f82bUL,
	iv3 = 0xa54ff53a5f1d36f1UL,
	iv4 = 0x510e527fade682d1UL,
	iv5 = 0x9b05688c2b3e6c1fUL,
	iv6 = 0x1f83d9abfb41bd6bUL,
	iv7 = 0x5be0cd19137e2179UL,
	nano_xor_iv0 = 0x6a09e667f2bdc900, // iv1 ^ 0x1010000 ^ outlen
	nano_xor_iv4 = 0x510e527fade682f9UL, // iv4 ^ inbytes
 	nano_xor_iv6 = 0xe07c265404be4294, // iv6 ^ ~0
};

#ifdef  cl_amd_media_ops
#pragma OPENCL EXTENSION cl_amd_media_ops : enable
static inline ulong rotr64(ulong x, int shift)
{
    uint2 x2 = as_uint2(x);
    if (shift < 32)
        return as_ulong(amd_bitalign(x2.s10, x2, shift));
    return as_ulong(amd_bitalign(x2, x2.s10, (shift - 32)));
}
#else
static inline ulong rotr64(ulong a, int shift) { return rotate(a, 64UL - shift); }
#endif

#define G32(m0, m1, m2, m3, vva, vb1, vb2, vvc, vd1, vd2) \
  do {                                                    \
    vva += (ulong2) (vb1 + m0, vb2 + m2);                 \
    vd1 = rotr64(vd1 ^ vva.s0, 32);                       \
    vd2 = rotr64(vd2 ^ vva.s1, 32);                       \
    vvc += (ulong2) (vd1, vd2);                           \
    vb1 = rotr64(vb1 ^ vvc.s0, 24);                       \
    vb2 = rotr64(vb2 ^ vvc.s1, 24);                       \
    vva += (ulong2) (vb1 + m1, vb2 + m3);                 \
    vd1 = rotr64(vd1 ^ vva.s0, 16);                       \
    vd2 = rotr64(vd2 ^ vva.s1, 16);                       \
    vvc += (ulong2) (vd1, vd2);                           \
    vb1 = rotr64(vb1 ^ vvc.s0, 63);                       \
    vb2 = rotr64(vb2 ^ vvc.s1, 63);                       \
  } while (0)

#define G2v(m0, m1, m2, m3, a, b, c, d) \
  G32(m0, m1, m2, m3, vv[a/2], vv[b/2].s0, vv[b/2].s1, vv[c/2], vv[d/2].s0, vv[d/2].s1)

#define G2v_split(m0, m1, m2, m3, a, vb1, vb2, c, vd1, vd2) \
  G32(m0, m1, m2, m3, vv[a/2], vb1, vb2, vv[c/2], vd1, vd2)

#define ROUND(m0, m1, m2, m3, m4, m5, m6, m7, m8, m9, m10, m11, m12, m13, m14, m15)         \
  do {                                                                                      \
    G2v     (m0, m1, m2, m3, 0, 4,  8, 12);                                                 \
    G2v     (m4, m5, m6, m7, 2, 6, 10, 14);                                                 \
    G2v_split(m8, m9, m10, m11, 0, vv[5/2].s1, vv[6/2].s0, 10, vv[15/2].s1, vv[12/2].s0);   \
    G2v_split(m12, m13, m14, m15, 2, vv[7/2].s1, vv[4/2].s0,  8, vv[13/2].s1, vv[14/2].s0); \
  } while(0)

static inline ulong blake2b(ulong const nonce, ulong4 const hash)
{
  ulong2 vv[8] = {
    { nano_xor_iv0, iv1 },
    { iv2, iv3 },
    { iv4, iv5 },
    { iv6, iv7 },
    { iv0, iv1 },
    { iv2, iv3 },
    { nano_xor_iv4, iv5 },
    { nano_xor_iv6, iv7 },
  };
  ulong *h = &hash;

  ROUND(nonce, h[0], h[1], h[2], h[3], 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0);
  ROUND(0, 0, h[3], 0, 0, 0, 0, 0, h[0], 0, nonce, h[1], 0, 0, 0, h[2]);
  ROUND(0, 0, 0, nonce, 0, h[1], 0, 0, 0, 0, h[2], 0, 0, h[0], 0, h[3]);
  ROUND(0, 0, h[2], h[0], 0, 0, 0, 0, h[1], 0, 0, 0, h[3], nonce, 0, 0);
  ROUND(0, nonce, 0, 0, h[1], h[3], 0, 0, 0, h[0], 0, 0, 0, 0, h[2], 0);
  ROUND(h[1], 0, 0, 0, nonce, 0, 0, h[2], h[3], 0, 0, 0, 0, 0, h[0], 0);
  ROUND(0, 0, h[0], 0, 0, 0, h[3], 0, nonce, 0, 0, h[2], 0, h[1], 0, 0);
  ROUND(0, 0, 0, 0, 0, h[0], h[2], 0, 0, nonce, 0, h[3], 0, 0, h[1], 0);
  ROUND(0, 0, 0, 0, 0, h[2], nonce, 0, 0, h[1], 0, 0, h[0], h[3], 0, 0);
  ROUND(0, h[1], 0, h[3], 0, 0, h[0], 0, 0, 0, 0, 0, h[2], 0, 0, nonce);
  ROUND(nonce, h[0], h[1], h[2], h[3], 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0);
  ROUND(0, 0, h[3], 0, 0, 0, 0, 0, h[0], 0, nonce, h[1], 0, 0, 0, h[2]);

  return nano_xor_iv0 ^ vv[0].s0 ^ vv[4].s0;
}
#undef G32
#undef G2v
#undef G2v_split
#undef ROUND

__kernel void nano_work (__constant ulong * attempt,
                         __global ulong * restrict result_a,
                         __constant ulong * item_a,
                         __constant ulong * difficulty_a,
                         __global ulong * restrict result_hash_a)
{
    const ulong attempt_l = *attempt + get_global_id(0);
    const ulong result = blake2b(attempt_l, vload4(0, item_a));
    if (result >= *difficulty_a) {
        *result_a = attempt_l;
        *result_hash_a = result;
    }
}`
