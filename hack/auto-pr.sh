#!/usr/bin/env bash
#
# auto-pr.sh opens (or updates) a pull request from develop into main. The PR
# body is generated from the conventional commit messages on the branch, with
# no external AI dependency. It expects the gh CLI to be authenticated.
set -euo pipefail

base="${BASE_BRANCH:-main}"
head="${HEAD_BRANCH:-develop}"

git fetch origin "$base" "$head" --quiet

commits=$(git log "origin/${base}..origin/${head}" --pretty=format:'%s' || true)
if [ -z "$commits" ]; then
  echo "No commits between ${base} and ${head}; nothing to do."
  exit 0
fi

section() {
  local prefix="$1" title="$2" matched
  matched=$(echo "$commits" | grep -E "^${prefix}(\(.+\))?!?: " || true)
  if [ -n "$matched" ]; then
    echo "### ${title}"
    echo "$matched" | sed -E "s/^${prefix}(\(.+\))?!?: /- /"
    echo
  fi
}

{
  echo "Automated pull request from \`${head}\` into \`${base}\`."
  echo
  section "feat" "Features"
  section "fix" "Fixes"
  section "perf" "Performance"
  section "docs" "Documentation"
  section "refactor" "Refactoring"
  section "chore" "Chores"
} > pr-body.md

title="Release: merge ${head} into ${base}"

existing=$(gh pr list --base "$base" --head "$head" --state open --json number --jq '.[0].number // empty' || true)
if [ -n "$existing" ]; then
  gh pr edit "$existing" --title "$title" --body-file pr-body.md
  echo "Updated pull request #${existing}"
else
  gh pr create --base "$base" --head "$head" --title "$title" --body-file pr-body.md
  echo "Created pull request"
fi
