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

func NewWorkerGPU() (*clWorker, error) {
	return NewWorkerGPUThread(1 << 23)
}

func NewWorkerGPUThread(thread uint64) (*clWorker, error) {
	device, err := getDevice()
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

func getDevice() (dv opencl.Device, err error) {
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
enum blake2b_constant
{
	BLAKE2B_BLOCKBYTES = 128,
	BLAKE2B_OUTBYTES   = 64,
	BLAKE2B_KEYBYTES   = 64,
	BLAKE2B_SALTBYTES  = 16,
	BLAKE2B_PERSONALBYTES = 16
};
typedef struct __blake2b_param
{
	uchar  digest_length; // 1
	uchar  key_length;    // 2
	uchar  fanout;        // 3
	uchar  depth;         // 4
	uint leaf_length;   // 8
	ulong node_offset;   // 16
	uchar  node_depth;    // 17
	uchar  inner_length;  // 18
	uchar  reserved[14];  // 32
	uchar  salt[BLAKE2B_SALTBYTES]; // 48
	uchar  personal[BLAKE2B_PERSONALBYTES];  // 64
} blake2b_param;

// Optimize: blake2 can overflow a uint64_t only if messages larger than
// 17 exabytes are seen. Set the value 2 if messages larger than 17 exabytes
// should be supported, otherwise keep 1.
#define BLAKE2B_LONG_MESSAGE (1)

typedef struct __blake2b_state
{
	ulong h[8];
	ulong t[BLAKE2B_LONG_MESSAGE];
	ulong f[1];
	uchar  buf[2 *  BLAKE2B_BLOCKBYTES];
	ushort buflen;
} blake2b_state;

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
};

static inline int blake2b_increment_counter( blake2b_state *S, const ushort inc )
{
  S->t[0] += inc;
#if BLAKE2B_LONG_MESSAGE > 1
  S->t[1] += ( S->t[0] < inc );
#endif
  return 0;
}
static inline ulong load64( const void *src )
{
#if defined(__ENDIAN_LITTLE__)
  return *( ulong * )( src );
#else
  const uchar *p = ( uchar * )src;
  ulong w = *p++;
  w |= ( ulong )( *p++ ) <<  8;
  w |= ( ulong )( *p++ ) << 16;
  w |= ( ulong )( *p++ ) << 24;
  w |= ( ulong )( *p++ ) << 32;
  w |= ( ulong )( *p++ ) << 40;
  w |= ( ulong )( *p++ ) << 48;
  w |= ( ulong )( *p++ ) << 56;
  return w;
#endif
}

static inline void store64( void *dst, ulong w )
{
#if defined(__ENDIAN_LITTLE__)
  *( ulong * )( dst ) = w;
#else
  uchar *p = ( uchar * )dst;
  *p++ = ( uchar )w; w >>= 8;
  *p++ = ( uchar )w; w >>= 8;
  *p++ = ( uchar )w; w >>= 8;
  *p++ = ( uchar )w; w >>= 8;
  *p++ = ( uchar )w; w >>= 8;
  *p++ = ( uchar )w; w >>= 8;
  *p++ = ( uchar )w; w >>= 8;
  *p++ = ( uchar )w;
#endif
}

static inline void memzero_u32(uint *dest, size_t count)
{
  for (ushort i = 0; i < count / sizeof(uint); ++i)
    dest[i] = 0;
}

static void blake2b_init( blake2b_state *S, const uchar outlen )
{
  blake2b_param P[1];
  P->digest_length = outlen;
  P->key_length    = 0;
  P->fanout        = 1;
  P->depth         = 1;
  P->leaf_length   = 0;

  /* init XORs IV with input parameter block */
  ulong *p = ( ulong * ) P;
  S->h[0] = iv0 ^ p[0];
  S->h[1] = iv1;
  S->h[2] = iv2; S->h[3] = iv3;
  S->h[4] = iv4; S->h[5] = iv5;
  S->h[6] = iv6; S->h[7] = iv7;
  memzero_u32( (uchar *) S + sizeof(S->h), sizeof(blake2b_state) - sizeof(S->h) );
}

static inline ulong rotr64(ulong a, ulong shift) { return rotate(a, 64 - shift); }

#define G32(s, vva, vb1, vb2, vvc, vd1, vd2)                      \
  do {                                                            \
    vva += (ulong2) (vb1 + m[s >> 24], vb2 + m[(s >> 8) & 0xf]);  \
    vd1 = rotr64(vd1 ^ vva.s0, 32lu);                             \
    vd2 = rotr64(vd2 ^ vva.s1, 32lu);                             \
    vvc += (ulong2) (vd1, vd2);                                   \
    vb1 = rotr64(vb1 ^ vvc.s0, 24lu);                             \
    vb2 = rotr64(vb2 ^ vvc.s1, 24lu);                             \
    vva += (ulong2) (vb1 + m[(s >> 16) & 0xf], vb2 + m[s & 0xf]); \
    vd1 = rotr64(vd1 ^ vva.s0, 16lu);                             \
    vd2 = rotr64(vd2 ^ vva.s1, 16lu);                             \
    vvc += (ulong2) (vd1, vd2);                                   \
    vb1 = rotr64(vb1 ^ vvc.s0, 63lu);                             \
    vb2 = rotr64(vb2 ^ vvc.s1, 63lu);                             \
  } while (0)

#define G2v(s, a, b, c, d) \
  G32(s, vv[a/2], vv[b/2].s0, vv[b/2].s1, vv[c/2], vv[d/2].s0, vv[d/2].s1)

#define G2vsplit(s, a, vb1, vb2, c, vd1, vd2) \
  G32(s, vv[a/2], vb1, vb2, vv[c/2], vd1, vd2)

#define ROUND(sig0, sig1, sig2, sig3)                                        \
  do {                                                                       \
    G2v     (sig0, 0, 4,  8, 12);                                            \
    G2v     (sig1, 2, 6, 10, 14);                                            \
    G2vsplit(sig2, 0, vv[5/2].s1, vv[6/2].s0, 10, vv[15/2].s1, vv[12/2].s0); \
    G2vsplit(sig3, 2, vv[7/2].s1, vv[4/2].s0,  8, vv[13/2].s1, vv[14/2].s0); \
  } while(0)

static void blake2b_compress( blake2b_state *S )
{
  ulong m[16];
  int i;

  #pragma unroll 16
  for (i = 0; i < 16; i++)
      m[i] = load64(S->buf + i * sizeof(m[0]));

  ulong2 vv[8] = {
    { S->h[0], S->h[1] },
    { S->h[2], S->h[3] },
    { S->h[4], S->h[5] },
    { S->h[6], S->h[7] },
    { iv0, iv1 },
    { iv2, iv3 },
    { iv4 ^ S->t[0],
#if BLAKE2B_LONG_MESSAGE > 1
      iv5 ^ S->t[1] },
#else
      iv5 },
#endif
    { iv6 ^ S->f[0],
      iv7 /* ^ S->f[1] is removed because no last_node */ },
  };

  ROUND(0x00010203, 0x04050607, 0x08090a0b, 0x0c0d0e0f);
  ROUND(0x0e0a0408, 0x090f0d06, 0x010c0002, 0x0b070503);
  ROUND(0x0b080c00, 0x05020f0d, 0x0a0e0306, 0x07010904);
  ROUND(0x07090301, 0x0d0c0b0e, 0x0206050a, 0x04000f08);
  ROUND(0x09000507, 0x02040a0f, 0x0e010b0c, 0x0608030d);
  ROUND(0x020c060a, 0x000b0803, 0x040d0705, 0x0f0e0109);
  ROUND(0x0c05010f, 0x0e0d040a, 0x00070603, 0x0902080b);
  ROUND(0x0d0b070e, 0x0c010309, 0x05000f04, 0x0806020a);
  ROUND(0x060f0e09, 0x0b030008, 0x0c020d07, 0x01040a05);
  ROUND(0x0a020804, 0x07060105, 0x0f0b090e, 0x030c0d00);
  ROUND(0x00010203, 0x04050607, 0x08090a0b, 0x0c0d0e0f);
  ROUND(0x0e0a0408, 0x090f0d06, 0x010c0002, 0x0b070503);

  #pragma unroll 4
  for (i = 0; i < 4; ++i) {
      ulong2 x = vv[i] ^ vv[i + 4];
      S->h[i*2  ] ^= x.s0;
      S->h[i*2+1] ^= x.s1;
  }
}
#undef G32
#undef G2v
#undef G2vsplit
#undef ROUND

static inline void memcpy_u64(ulong *dst, ulong const *src, ushort count)
{
    for (ushort i = 0; i < count / sizeof(ulong); i++)
        *dst++ = *src++;
}

/* inlen now in bytes */
static void blake2b_update( blake2b_state *S, const uchar *in, ushort inlen )
{
  while( inlen > 0 )
  {
	size_t left = S->buflen;
	size_t fill = 2 * BLAKE2B_BLOCKBYTES - left;
	if( inlen > fill )
	{
	  memcpy_u64( S->buf + left, in, fill ); // Fill buffer
	  S->buflen += fill;
	  blake2b_increment_counter( S, BLAKE2B_BLOCKBYTES );
	  blake2b_compress( S );
	  S->buflen -= BLAKE2B_BLOCKBYTES;
	  in += fill;
	  inlen -= fill;
	}
	else // inlen <= fill
	{
	  memcpy_u64( S->buf + left, in, inlen );
	  S->buflen += inlen; // Be lazy, do not compress
	  break;
	}
  }
}

static void blake2b_final( blake2b_state *S, uchar *out, ushort outlen )
{
  blake2b_increment_counter( S, S->buflen );

  S->f[0] = ~0UL; // set last block
  blake2b_compress( S );

  // assume outlen is multiple of sizeof(ulong)
  for( int i = 0; i < outlen/sizeof(ulong); ++i )
      store64( out + sizeof(S->h[0]) * i, S->h[i] );
}

static inline void memcpy_u64_global(ulong *dst, __global ulong const *src, ushort count)
{
    #pragma unroll 4
    for (ushort i = 0; i < count / sizeof(ulong); i++)
        *dst++ = *src++;
}

__kernel void nano_work (__global ulong const * attempt, __global ulong * result_a, __global uchar const * item_a, __global ulong const * difficulty_a, __global ulong * result_hash_a)
{
	int const thread = get_global_id (0);
	uchar item_l [32];
	memcpy_u64_global(item_l, item_a, 32);
	ulong attempt_l = *attempt + thread;
	blake2b_state state;
	blake2b_init (&state, sizeof (ulong));
	blake2b_update (&state, (uchar *) &attempt_l, sizeof (ulong));
	blake2b_update (&state, item_l, 32);
	ulong result;
	blake2b_final (&state, (uchar *) &result, sizeof (result));
	if (result >= * difficulty_a)
	{
		*result_a = attempt_l;
		*result_hash_a = result;
	}
}`
