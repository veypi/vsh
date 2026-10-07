#!/usr/bin/env bash
#
# vsh 发布打 tag：根模块 + contrib/<name> 子模块。
#
# 版本约定（详见根仓 README「发布」节）：
#   - 根模块 tag：vX.Y.Z
#   - contrib 模块 tag：contrib/<name>/vX.Y.Z —— 各自独立版本，均自 v0.1.0 起
#   - 根模块升 minor 时，所有 contrib 必须跟版：各自发一版，并把
#     require github.com/veypi/vsh 指到新的根版本
#
# 用法：
#   scripts/release-tags.sh --all 0.2.0                # 根 v0.2.0 + 所有 contrib v0.2.0
#   scripts/release-tags.sh --root 0.2.1               # 只给根打 tag
#   scripts/release-tags.sh --only jq=0.1.1 yq=0.1.2   # 只给指定 contrib 打 tag
#   scripts/release-tags.sh --list                     # 列出 contrib 模块与已有 tag
#   ……任意模式加 --yes 才真正创建 tag；默认只打印将执行的命令（dry-run）。
#
# 脚本只创建本地 tag，不推送。推送请显式执行：git push origin <tag>
set -euo pipefail

cd "$(cd "$(dirname "$0")/.." && pwd)"

mode=""
rootver=""
contribver=""
only=""
yes=0

modules() {
  for d in contrib/*/; do
    [ -f "$d/go.mod" ] || continue
    basename "$d"
  done
}

is_module() {
  [ -f "contrib/$1/go.mod" ]
}

valid_version() {
  printf '%s' "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'
}

create_tag() {
  tag="$1"
  if git rev-parse -q --verify "refs/tags/$tag" >/dev/null 2>&1; then
    echo "  跳过 $tag（已存在）"
    return 0
  fi
  if [ "$yes" -eq 1 ]; then
    git tag "$tag"
    echo "  已创建 $tag"
  else
    echo "  git tag $tag    （dry-run；加 --yes 执行）"
  fi
}

usage() {
  cat <<'USAGE'
vsh 发布打 tag：根模块 + contrib/<name> 子模块。

版本约定（详见 README「发布」节）：
  根模块 tag          vX.Y.Z
  contrib 模块 tag    contrib/<name>/vX.Y.Z（各自独立版本，均自 v0.1.0 起）
  根模块升 minor     所有 contrib 必须跟版：各自发一版，并把
                     require github.com/veypi/vsh 指到新的根版本

用法：
  scripts/release-tags.sh --all 0.2.0                # 根 v0.2.0 + 所有 contrib v0.2.0
  scripts/release-tags.sh --root 0.2.1               # 只给根打 tag
  scripts/release-tags.sh --contrib 0.1.0            # 所有 contrib 打同一版本，根不动
  scripts/release-tags.sh --only jq=0.1.1 yq=0.1.2   # 只给指定 contrib 打 tag
  scripts/release-tags.sh --list                     # 列出 contrib 模块与已有 tag

任意模式加 --yes 才真正创建 tag；默认只打印将执行的命令（dry-run）。
脚本只创建本地 tag，不推送；推送请显式执行 git push origin <tag>。
USAGE
}

case "${1:-}" in
  ""|-h|--help|help)
    usage
    exit 0
    ;;
esac

while [ $# -gt 0 ]; do
  case "$1" in
    --all)  mode=all;  rootver="${2:-}"; shift 2 ;;
    --root) mode=root; rootver="${2:-}"; shift 2 ;;
    --contrib) mode=contrib; contribver="${2:-}"; shift 2 ;;
    --only) mode=only; shift ;;
    --list) mode=list; shift ;;
    --yes)  yes=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *)
      if [ "$mode" = only ]; then
        only="$only $1"
      else
        echo "未知参数：$1（用 --help 看用法）" >&2
        exit 2
      fi
      shift
      ;;
  esac
done

case "$mode" in
  list)
    echo "根模块：$(cat VERSION 2>/dev/null || echo '?')"
    echo "contrib 模块（$(modules | wc -l | tr -d ' ') 个）："
    for m in $(modules); do
      tags="$(git tag -l "contrib/$m/*" | tr '\n' ' ')"
      printf '  %-16s %s\n' "$m" "${tags:-（未发版）}"
    done
    exit 0
    ;;

  root)
    valid_version "$rootver" || { echo "版本号格式不对：$rootver（应为 X.Y.Z）" >&2; exit 2; }
    echo "根模块："
    create_tag "v$rootver"
    ;;

  all)
    valid_version "$rootver" || { echo "版本号格式不对：$rootver（应为 X.Y.Z）" >&2; exit 2; }
    echo "根模块："
    create_tag "v$rootver"
    echo "contrib 模块："
    for m in $(modules); do
      create_tag "contrib/$m/v$rootver"
    done
    ;;

  contrib)
    valid_version "$contribver" || { echo "版本号格式不对：$contribver（应为 X.Y.Z）" >&2; exit 2; }
    echo "contrib 模块："
    for m in $(modules); do
      create_tag "contrib/$m/v$contribver"
    done
    ;;

  only)
    [ -n "$only" ] || { echo "--only 需要 name=version 参数" >&2; exit 2; }
    for item in $only; do
      name="${item%%=*}"
      ver="${item#*=}"
      if [ "$name" = "$item" ] || [ -z "$ver" ]; then
        echo "参数格式应为 name=version：$item" >&2
        exit 2
      fi
      is_module "$name" || { echo "未知 contrib 模块：$name" >&2; exit 2; }
      valid_version "$ver" || { echo "版本号格式不对：$ver（应为 X.Y.Z）" >&2; exit 2; }
      create_tag "contrib/$name/v$ver"
    done
    ;;

  *)
    echo "需要 --all / --root / --only / --list 之一（用 --help 看用法）" >&2
    exit 2
    ;;
esac

if [ "$yes" -eq 1 ]; then
  echo
  echo "本地 tag 已创建。推送（按需）："
  echo "  git push origin --tags     # 或逐个 git push origin <tag>"
else
  echo
  echo "以上为 dry-run；加 --yes 实际创建本地 tag。"
fi
