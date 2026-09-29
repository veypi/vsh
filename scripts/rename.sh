#!/bin/zsh
# vsh fork rename script — re-runnable (idempotent).
# Baseline: github.com/veypi/vsh @ 88728c5a0618cf8d8278a6602ae9e1cf05a2159d
# Renames: module path -> github.com/veypi/vsh, identifiers vsh->vsh / VSH->VSH / Vsh->Vsh,
# and files/dirs with 'vsh' in their name.
# Usage: scripts/rename.sh [repo_root]   (default: parent of this script)
# Note: ./docs is excluded (todo is ours; the old design/AST_ROADMAP files were removed).
set -euo pipefail
ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
cd "$ROOT"

files=$(grep -rlI -e 'vsh' -e 'VSH' -e 'Vsh' . --exclude-dir=.git --exclude-dir=docs || true)

if [ -n "${files// /}" ]; then
  # 1) full module path first (order matters), 2) bare identifiers
  print -l -- ${(f)files} | xargs perl -pi -e \
    's{github\.com/ewhauser/vsh}{github.com/veypi/vsh}g; s/vsh/vsh/g; s/VSH/VSH/g; s/Vsh/Vsh/g'
fi

# 3) rename files/dirs containing vsh in the name (deepest first)
find . -depth -name '*vsh*' -not -path './.git/*' | while read -r f; do
  base=$(basename "$f")
  newbase=$(print -- "$base" | perl -pe 's/vsh/vsh/g')
  mv -- "$f" "$(dirname "$f")/$newbase"
done

echo "rename done"
