//go:build darwin && arm64

package memory

const ortLibName = "libonnxruntime.dylib"
const ortDownloadURL = "https://github.com/microsoft/onnxruntime/releases/download/v1.24.4/onnxruntime-osx-arm64-1.24.4.tgz"
const ortArchiveLibPath = "onnxruntime-osx-arm64-1.24.4/lib/libonnxruntime.1.24.4.dylib"
const ortArchiveFormat = "tgz"
