//go:build !unix

package main

import "errors"

// errTextBusy：非 unix 平台没有 ETXTBSY；用一个永不匹配的哨兵，
// 于是重试循环第一次就返回（语义上这些平台不会出现该竞态）。
var errTextBusy = errors.New("ETXTBSY (not applicable on this platform)")
