#!/usr/bin/env bash
# Explicit, read-only source checks. No installs, rewrites, hooks or services.
set -uo pipefail
usage() {
  cat <<'USAGE'
Usage: scripts/check.sh [--phase all|quality|go|relay|commits] [--range BASE..HEAD]
Default: all (requires --range). quality runs Go and relay checks only.
Commits require an explicit nonempty two-dot ancestor range; both endpoints
must resolve to commits. All subjects in that range must be Conventional Commits.
Exit: 0 passed; 1 failed/missing checks; 2 invalid usage or source/range.
Run under your host's execution policy. No dependency install or source rewrite.
USAGE
}
phase=all
range=
seen_phase=false
seen_range=false
while (($#)); do
  case "$1" in
    --help|-h)
      usage
      exit 0 ;;
    --phase)
      if [[ $# -lt 2 || $seen_phase == true ]]; then
        usage >&2
        exit 2
      fi
      phase=$2
      seen_phase=true
      shift 2 ;;
    --range)
      if [[ $# -lt 2 || $seen_range == true ]]; then
        usage >&2
        exit 2
      fi
      range=$2
      seen_range=true
      shift 2 ;;
    *)
      usage >&2
      exit 2 ;;
  esac
done
case "$phase" in
  all|quality|go|relay|commits) ;;
  *)
    usage >&2
    exit 2 ;;
esac
if [[ $phase == all || $phase == commits ]]; then
  if [[ -z $range ]]; then
    usage >&2
    exit 2
  fi
elif [[ $seen_range == true ]]; then
  echo '--range is only valid with all or commits' >&2
  exit 2
fi
if ! command -v git >/dev/null; then
  echo 'Missing prerequisite: git' >&2
  exit 1
fi
# Locate source from this script, never from a caller-supplied working directory.
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd) || exit 2
cd -- "$root" || exit 2
git_root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 2
if [[ $git_root != "$root" ]]; then
  echo "Script must live at the repository root/scripts" >&2
  exit 2
fi
head=$(git rev-parse --verify HEAD) || exit 2
if [[ -n $range ]]; then
  # Reject option injection, symmetric ranges, implicit endpoints and revision sets.
  if [[ ! $range =~ ^([A-Za-z0-9_][A-Za-z0-9_./~^{}@-]*)\.\.([A-Za-z0-9_][A-Za-z0-9_./~^{}@-]*)$ ]]; then
    echo 'Invalid range: use explicit BASE..HEAD commit names' >&2
    exit 2
  fi
  base_name=${BASH_REMATCH[1]}
  tip_name=${BASH_REMATCH[2]}
  if [[ $base_name == *..* || $tip_name == *..* ]]; then
    echo "Invalid range endpoint" >&2
    exit 2
  fi
  base=$(git rev-parse --verify --end-of-options "$base_name^{commit}" 2>/dev/null) || exit 2
  tip=$(git rev-parse --verify --end-of-options "$tip_name^{commit}" 2>/dev/null) || exit 2
  if ! git merge-base --is-ancestor "$base" "$tip"; then
    echo "Range base must be an ancestor" >&2
    exit 2
  fi
  if [[ $base == "$tip" ]]; then
    echo "Empty commit range" >&2
    exit 2
  fi
fi
printf 'Source HEAD: %s\nPhase: %s\n' "$head" "$phase"
git --version
echo 'Working tree (checks include current edits; HEAD alone is not patch identity):'
git status --short --untracked-files=all
scratch=$(mktemp -d "/tmp/capsule-check.XXXXXXXX") || exit 1
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
failed=0
run() {
  local label=$1 code
  shift
  printf '\nCHECK %s\n' "$label"
  "$@"
  code=$?
  printf 'RESULT %s: exit %s\n' "$label" "$code"
  if ((code != 0)); then
    failed=1
  fi
}
need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'MISSING prerequisite: %s\n' "$1" >&2
    failed=1
    return 1
  fi
}
if [[ $phase == all || $phase == quality || $phase == go ]]; then
  # NUL paths preserve whitespace, newlines and metacharacters. Prefix ./ keeps
  # dash-leading filenames from becoming gofmt options. Do not batch into argv.
  if need go && need gofmt && need cp && need cmp; then
    # Never auto-download a toolchain or modules during an inspection gate.
    export GOTOOLCHAIN=local
    export GOPROXY=off
    export GONOPROXY=none
    printf 'Go executable: %s\nGofmt executable: %s\n' "$(command -v go)" "$(command -v gofmt)"
    run 'Go tool identity' go version
    run 'Go environment' go env GOVERSION GOOS GOARCH GOMOD
    git ls-files --cached --others --exclude-standard -z -- '*.go' > "$scratch/go-files" || exit 2
    count=0
    format_failed=0
    while IFS= read -r -d '' file; do
      [[ -f $file ]] || continue
      count=$((count + 1))
      if ! gofmt -l "./$file" > "$scratch/gofmt-result"; then
        format_failed=1
      fi
      if [[ -s $scratch/gofmt-result ]]; then
        printf 'Unformatted Go path: %q\n' "$file"
        format_failed=1
      fi
    done < "$scratch/go-files"
    if ((count == 0)); then
      echo 'FAIL: no Go source found'
      format_failed=1
    fi
    printf 'RESULT Go formatting (%s files): exit %s\n' "$count" "$format_failed"
    if ((format_failed)); then
      failed=1
    fi
    # Ignore ambient GOFLAGS (which could select -mod=mod or output paths).
    # Package/test concurrency is bounded. Test binaries and build outputs are
    # temporary; no executable is emitted into the checkout even for one main.
    # -mod=readonly protects go.mod but can still add go.sum entries. Use
    # disposable manifest copies and fail if either would have changed.
    if cp go.mod "$scratch/go.mod"; then
      if [[ -f go.sum ]]; then
        cp go.sum "$scratch/go.sum" || exit 1
      fi
      run 'Go vet' env GOFLAGS= GOMAXPROCS=2 go vet -modfile="$scratch/go.mod" -mod=readonly -p=2 ./...
      run 'Go test' env GOFLAGS= GOMAXPROCS=2 go test -modfile="$scratch/go.mod" -mod=readonly -p=2 -timeout=120s ./...
      run 'Go build' env GOFLAGS= GOMAXPROCS=2 go build -modfile="$scratch/go.mod" -mod=readonly -p=2 -o "$scratch/" ./...
      if ! cmp -s go.mod "$scratch/go.mod"; then
        echo 'FAIL: Go checks changed the disposable go.mod'
        failed=1
      fi
      if [[ -f go.sum ]]; then
        if ! cmp -s go.sum "$scratch/go.sum"; then
          echo 'FAIL: Go checks require go.sum changes'
          failed=1
        fi
      elif [[ -s $scratch/go.sum ]]; then
        echo 'FAIL: Go checks require a go.sum'
        failed=1
      fi
    else
      echo 'FAIL: missing/unreadable go.mod'
      failed=1
    fi
  fi
fi
if [[ $phase == all || $phase == quality || $phase == relay ]]; then
  if need node && need npm; then
    printf 'Node executable: %s\nnpm executable: %s\n' "$(command -v node)" "$(command -v npm)"
    run 'Node tool identity' node --version
    run 'npm tool identity' npm --version
    if [[ -x cloudflare/node_modules/.bin/tsc ]]; then
      printf 'TypeScript executable: %s/cloudflare/node_modules/.bin/tsc\n' "$root"
      run 'TypeScript tool identity' cloudflare/node_modules/.bin/tsc --version
      run 'Relay no-emit typecheck' cloudflare/node_modules/.bin/tsc --noEmit -p cloudflare/tsconfig.json
    else
      echo 'MISSING prerequisite: local TypeScript (use reviewed npm ci separately)'
      failed=1
    fi
    if [[ -x cloudflare/node_modules/.bin/prettier ]]; then
      printf 'Prettier executable: %s/cloudflare/node_modules/.bin/prettier\n' "$root"
      run 'Prettier tool identity' cloudflare/node_modules/.bin/prettier --version
      # Run in relay directory so config lookup and paths match npm scripts.
      (cd cloudflare && ./node_modules/.bin/prettier --check 'src/**/*.ts' package.json tsconfig.json .prettierrc.json)
      code=$?
      printf 'RESULT Relay formatting: exit %s\n' "$code"
      if ((code)); then
        failed=1
      fi
    else
      echo 'MISSING prerequisite: local Prettier (use reviewed npm ci separately)'
      failed=1
    fi
  fi
fi
if [[ $phase == all || $phase == commits ]]; then
  printf '\nCommit range: %s..%s\n' "$base" "$tip"
  git log --format='%H%x00%s%x00' -z "$base..$tip" > "$scratch/commits" || exit 2
  # Accept standard project types with optional scope and breaking marker.
  subject_pattern='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^()[:cntrl:]]+\))?!?: [^[:space:]].*$'
  while IFS= read -r -d '' sha && IFS= read -r -d '' subject && IFS= read -r -d '' separator; do
    if [[ ! $subject =~ $subject_pattern ]]; then
      printf 'FAIL commit %s subject: %q\n' "$sha" "$subject"
      failed=1
    else
      printf 'PASS commit %s subject: %q\n' "$sha" "$subject"
    fi
  done < "$scratch/commits"
else
  echo 'NOT RUN: commit subjects (select all or commits with --range BASE..HEAD)'
fi
printf '\nGate exit: %s\n' "$failed"
exit "$failed"
