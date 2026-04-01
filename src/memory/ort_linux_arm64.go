//go:build linux && arm64

package memory

const ortLibName = "libonnxruntime.so"
const ortDownloadURL = "https://github.com/microsoft/onnxruntime/releases/download/v1.24.4/onnxruntime-linux-aarch64-1.24.4.tgz"
const ortArchiveLibPath = "onnxruntime-linux-aarch64-1.24.4/lib/libonnxruntime.so.1.24.4"
const ortArchiveFormat = "tgz"
