package commandutil

import (
	"context"
	"errors"
	"github.com/veypi/vsh/host"
	"io/fs"
	"path"
	"strings"
)

// CommandPath normalizes Windows command operands into the Unix-shaped FS namespace.
// POSIX names retain literal backslashes.
func CommandPath(name string, platform host.Platform) string {
	if platform.OS != host.OSWindows {
		return name
	}
	name = strings.ReplaceAll(name, `\`, "/")
	if len(name) >= 2 && name[1] == ':' && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) && (len(name) == 2 || name[2] == '/') {
		return path.Clean("/" + strings.ToLower(name[:1]) + "/" + name[2:])
	}
	return name
}

// CommandPaths is the sole PATH/PATHEXT search implementation. The supplied stat
// operates in the caller's filesystem namespace, never implicitly on the host.
func CommandPaths(ctx context.Context, name, cwd string, env map[string]string, platform host.Platform, all, requireExec bool, stat func(context.Context, string) (fs.FileInfo, error)) ([]string, error) {
	name = CommandPath(name, platform)
	explicit := strings.Contains(name, "/")
	var bases []string
	if explicit {
		bases = []string{name}
	} else if name != "" && env["PATH"] != "" {
		for entry := range strings.SplitSeq(env["PATH"], ":") {
			if entry == "" {
				entry = "."
			}
			bases = append(bases, path.Join(entry, name))
		}
	}
	exts := platform.PathExtensions
	if exts == nil {
		exts = platform.OS.PlatformDefaults().PathExtensions
	}
	if len(exts) > 0 && env["PATHEXT"] != "" {
		exts = nil
		for ext := range strings.SplitSeq(strings.ToLower(env["PATHEXT"]), ";") {
			ext = strings.TrimSpace(ext)
			if ext == "" {
				continue
			}
			if ext[0] != '.' {
				ext = "." + ext
			}
			exts = append(exts, ext)
		}
	}
	var found []string
	seen := map[string]bool{}
	var denied error
	for _, base := range bases {
		if !path.IsAbs(base) {
			base = path.Join(cwd, base)
		}
		candidates := []string{path.Clean(base)}
		if path.Ext(base) == "" {
			for _, ext := range exts {
				candidates = append(candidates, base+ext)
			}
		}
		for _, candidate := range candidates {
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			info, err := stat(ctx, candidate)
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) && explicit {
					// 显式路径的探测错误（如规则拒绝）直接上抛，让调用方报
					// 权限错误而非 not found；PATH 搜索形态下无法确认存在性，
					// 按 miss 继续搜，否则被拒目录会顶掉 command not found。
					denied = err
				}
				continue
			}
			if info.IsDir() || (requireExec && platform.RequiresExecutableBit() && info.Mode()&0111 == 0) {
				denied = fs.ErrPermission
				continue
			}
			found = append(found, candidate)
			if !all {
				return found, nil
			}
		}
	}
	if len(found) == 0 && denied != nil {
		return nil, denied
	}
	return found, nil
}
