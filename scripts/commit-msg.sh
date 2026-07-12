#!/usr/bin/env sh
commit_msg=$(cat "$1")

if ! echo "$commit_msg" | grep -qE "^(feat|fix|chore|docs|refactor|test|style|perf|ci)(\(.+\))?: .+"; then
  echo "Commit message must follow conventional commits format: type: description"
  echo "Got: $commit_msg"
  exit 1
fi