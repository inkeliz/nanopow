# Golang NanoPoW

NanoPoW is an implementation of the proof-of-work used by Nano. That supports CPU and GPU (currently only OpenCL).

## Usage

```golang
func main() {
    nanopow.GenerateWork([]byte{/** PREVIOUS HASH **/}, nanopow.V1BaseDifficult)
}
```

### CPU

By default, we always uses CPU. If GPU is available, it will use CPU and GPU, combined.

### GPU

By default, we don't support GPU. You need to enable it using the "build tags", when build use:

```bash
go build -tags cl
```

Currently the only available option is OpenCL ("cl" tag).

### Multiple GPUs

By default, we use the first detected opencl-compatible GPU. To change this or to use multiple GPUs you can create your own work pool.

Here's an example using all of the GPUs

```go
// Using github.com/inkeliz/go-opencl
platforms, err := opencl.GetPlatforms()
if err != nil {
    return nil, err
}

var platform opencl.Platform
var name string
pool := nanopow.NewPool()
for _, curPlatform := range platforms {
    err = curPlatform.GetInfo(opencl.PlatformName, &name)
    if err != nil {
        return nil, err
    }

    var devices []opencl.Device
    devices, err = curPlatform.GetDevices(opencl.DeviceTypeAll)
    if err != nil {
        return nil, err
    }

    for _, device := range devices {
		gpu, gpuErr := nanopow.NewWorkerGPU(device)
		if gpuErr == nil {
			pool.Workers = append(pool.Workers, gpu)
		} else {
			fmt.Printf("\n⚠️ Unable to use GPU %v", gpuErr)
		}
    }
}

// Generate work
pool.GenerateWork(hash, difficulty)
```

## "Benchmarks"

I don't have an huge amount of data to compare and limited devices to test against. Some devices doesn't supports OpenCL. All times are using the V1 difficulty.

| Device               | OS      | CPU-Only  | CPU + OpenCL |
| -------------------- | ------- | --------- | ------------ |
| Samsung Galaxy S20+  | Android | ~9.62 seg | _Untested_   |
| Blackberry KeyOne    | Android | ~36.7 seg | _Untested_   |
|                      |         |           |
| R9 3900X + RX 5700XT | Windows | ~0.66 seg | ~0.27 seg    |

## Limitations

#### Support OpenGL, OpenGLES or Vulkan:

Currently we only support OpenCL, which is not natively supported by some devices. I have some plans to add some support for OpenGL, OpenGLES or Vulkan. Seems that Vulkan Compute, if possible, is the best solution since it's compatible with Windows, Linux and Android.

## Contributing

Pull requests are welcome.

## License

[MIT](https://choosealicense.com/licenses/mit/)
