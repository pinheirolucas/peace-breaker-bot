#!/usr/bin/env bash
# PostToolUse hook: gofmt and go vet the edited Go file's package, and flag
# comments the repo's comment rule doesn't allow. Feedback goes to the agent.
set -u

input=$(cat)
file=$(printf '%s' "$input" | grep -o '"file_path"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)"$/\1/')

case "$file" in
  *.go) ;;
  *) exit 0 ;;
esac
[ -f "$file" ] || exit 0

cd "${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel)}" || exit 0
rel=${file#"$PWD"/}

problems=""

if unformatted=$(gofmt -l "$rel") && [ -n "$unformatted" ]; then
  problems+="gofmt: $rel is not formatted. Run: gofmt -w $rel"$'\n'
fi

if ! vet=$(go vet "./$(dirname "$rel")" 2>&1); then
  problems+="go vet ./$(dirname "$rel"):"$'\n'"$vet"$'\n'
fi

if git ls-files --error-unmatch "$rel" >/dev/null 2>&1; then
  added=$(git diff -U0 HEAD -- "$rel" | sed -n 's/^@@ -[0-9,]* +\([0-9]*\),\{0,1\}\([0-9]*\) @@.*/\1 \2/p')
else
  added="1 $(wc -l < "$rel")"
fi

comments=$(printf '%s\n' "$added" | awk -v file="$rel" '
  NF { start = $1; count = ($2 == "" ? 1 : $2); for (i = start; i < start + count; i++) add[i] = 1 }
  END {
    istest = file ~ /_test\.go$/
    while ((getline line < file) > 0) {
      n++
      lines[n] = line
    }
    for (i = 1; i <= n; i++) {
      if (!(i in add)) continue
      line = lines[i]
      if (line !~ /^[[:space:]]*\/\//) continue
      if (line ~ /^[[:space:]]*\/\/(go:|nolint|line |export )/ || line ~ /^\/\/ Package /) continue
      if (istest) { print i ": comment in a test file"; continue }
      if (line ~ /^[[:space:]]+\/\//) { print i ": comment inside a body"; continue }
      j = i
      while (j <= n && lines[j] ~ /^\/\//) j++
      decl = lines[j]
      if (decl ~ /^(func (\([^)]*\) )?|type |var |const )[a-z_]/) print i ": comment on an unexported identifier"
    }
  }' | sort -u -t: -k1,1n)

if [ -n "$problems" ]; then
  printf '%s' "$problems" >&2
  [ -n "$comments" ] && printf 'Comment rule (CLAUDE.md > Workflow) in %s:\n%s\n' "$rel" "$comments" >&2
  exit 2
fi

if [ -n "$comments" ]; then
  msg="Comment rule (CLAUDE.md > Workflow): only exported identifiers get a one-line doc comment; none in tests or bodies. Review these new comments in $rel and remove them unless the code truly can't say it:\n${comments//$'\n'/\\n}"
  printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"%s"}}\n' "${msg//\"/\\\"}"
fi

exit 0
