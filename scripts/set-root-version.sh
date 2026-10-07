#!/usr/bin/env bash
#
# 把每个 contrib 模块（以及 examples/）对根模块的 require 改到指定版本。
# 用于「全量跟版」：根模块升 minor 后，一条命令把各模块的 require 指到新根版本，
# 之后各自补 CHANGELOG 并打 tag（见 scripts/release-tags.sh）。
#
# 用法：
#   scripts/set-root-version.sh 0.3.0            # dry-run：只列出会改哪些模块
#   scripts/set-root-version.sh 0.3.0 --yes      # 实际改写 go.mod
#
# 只改 require 行，不动 replace（仓内开发仍指向本地 ../..）；间接依赖清理请在各模块
# 目录内自行执行 `GOWORK=off go mod tidy`。
set -euo pipefail

cd "$(cd "$(dirname "$0")/.." && pwd)"

ver="${1:-}"
yes="${2:-}"

if ! printf '%s' "$ver" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "用法：scripts/set-root-version.sh <X.Y.Z> [--yes]" >&2
  exit 2
fi

# 各模块 go.mod 里根模块的 require 版本（兼容单行 require 与 require 块两种形态）
root_require() {
  awk '/github\.com\/veypi\/vsh v/ {
         for (i = 1; i <= NF; i++) {
           if ($i ~ /^v[0-9]+\.[0-9]+\.[0-9]+$/) { print $i; exit }
         }
       }' "$1"
}

mods=""
for d in contrib/*/ examples/; do
  if [ -f "$d/go.mod" ]; then
    mods="$mods $d"
  fi
done

for d in $mods; do
  cur="$(root_require "$d/go.mod")"
  if [ "$cur" = "v$ver" ]; then
    printf '  %-24s 已是 v%s，跳过\n' "$d" "$ver"
    continue
  fi
  if [ "$yes" = "--yes" ]; then
    ( cd "$d" && GOWORK=off go mod edit "-require=github.com/veypi/vsh@v$ver" )
    printf '  %-24s %s → v%s\n' "$d" "${cur:-（无）}" "$ver"
  else
    printf '  %-24s %s → v%s   （dry-run）\n' "$d" "${cur:-（无）}" "$ver"
  fi
done

if [ "$yes" != "--yes" ]; then
  echo
  echo "以上为 dry-run；加 --yes 实际改写。"
fi
