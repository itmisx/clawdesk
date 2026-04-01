//go:build darwin && amd64

package memory

const ortLibName = "libonnxruntime.dylib"
const ortDownloadURL = "https://github.com/microsoft/onnxruntime/releases/download/v1.23.2/onnxruntime-osx-x86_64-1.23.2.tgz"
const ortArchiveLibPath = "onnxruntime-osx-x86_64-1.23.2/lib/libonnxruntime.1.23.2.dylib"
const ortArchiveFormat = "tgz"
