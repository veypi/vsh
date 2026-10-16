package builtins

import (
	"context"
	"fmt"
	"io"
	stdfs "io/fs"
	"os"
	"path"

	gbfs "github.com/veypi/vsh/fs"
)

func ensureParentDirExists(ctx context.Context, inv *Invocation, targetAbs string) error {
	parent := path.Dir(targetAbs)
	info, _, exists, err := statMaybe(ctx, inv, parent)
	if err != nil {
		return err
	}
	if !exists {
		return &ExitError{
			Code: 1,
			Err:  fmt.Errorf("%s: No such file or directory", parent),
		}
	}
	if !info.IsDir() {
		return &ExitError{
			Code: 1,
			Err:  fmt.Errorf("%s: Not a directory", parent),
		}
	}
	return nil
}

func copyFileContents(ctx context.Context, inv *Invocation, srcAbs, dstAbs string, perm stdfs.FileMode) error {
	if err := ensureParentDirExists(ctx, inv, dstAbs); err != nil {
		return err
	}

	srcFile, err := inv.FS.Open(ctx, srcAbs)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := inv.FS.OpenFile(ctx, dstAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	recordFileMutation(inv.TraceRecorder(), "copy", dstAbs, srcAbs, dstAbs)
	return nil
}

func writeFileContents(ctx context.Context, inv *Invocation, targetAbs string, data []byte, perm stdfs.FileMode) error {
	file, err := createFileForWrite(ctx, inv, targetAbs, perm)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Write(data); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	recordFileMutation(inv.TraceRecorder(), "write", targetAbs, targetAbs, targetAbs)
	return nil
}

// createFileForWrite 建好父目录并打开目标文件（O_CREATE|O_WRONLY|O_TRUNC）。
// 调用方负责 Close，并可向返回的 writer 流式写入（curl -o 的落盘路径——
// 响应体边收边写，不整块驻留内存）。
func createFileForWrite(ctx context.Context, inv *Invocation, targetAbs string, perm stdfs.FileMode) (gbfs.File, error) {
	if err := ensureParentDirExists(ctx, inv, targetAbs); err != nil {
		return nil, err
	}

	file, err := inv.FS.OpenFile(ctx, targetAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return nil, &ExitError{Code: 1, Err: err}
	}
	return file, nil
}
