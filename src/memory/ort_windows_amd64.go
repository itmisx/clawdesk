//go:build windows && amd64

package memory

const ortLibName = "onnxruntime.dll"
const ortDownloadURL = "https://github.com/microsoft/onnxruntime/releases/download/v1.24.4/onnxruntime-win-x64-1.24.4.zip"
const ortArchiveLibPath = "onnxruntime-win-x64-1.24.4/lib/onnxruntime.dll"
const ortArchiveFormat = "zip"
