//go:build plan9

package vsh

// vsh 的支持面是 darwin / linux / windows（见 README 「平台支持」）。plan9 缺
// POSIX errno 族（EPIPE/ELOOP/EROFS…）与 fork/pipe 语义，不做支持。
//
// 这个刻意不存在的 import 让构建在包边界上直接失败（报错即包名），
// 而不是在依赖链深处报一堆 undefined: syscall.XXX。仅在 plan9 下参与编译。
import _ "vsh/unsupported-platform-plan9-supported-are-darwin-linux-windows"
