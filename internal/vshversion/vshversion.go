// Package vshversion 提供 vsh 模块自身的版本号。
//
// 版本身份只由仓库根 VERSION 文件定义；本包的常量必须与之一致，
// vshversion_test.go 会在测试时校验（避免两处漂移）。
package vshversion

// Version 是 vsh 的发布版本（与仓库根 VERSION 文件同步）。
const Version = "0.3.1"
