#!/bin/sh
# harness-baseline.sh — tingly-box 阶段验收基线
#
# 用真实 agent CLI（claude / codex / opencode）跑一遍 tingly-box gateway 数据面：
#   默认        mock 冒烟（vmodel 虚拟上游，零配置、零外部依赖）
#   --real FILE mock 之后追加真实 provider 扫描（需要 providers.yaml + 真实凭证）
#
# 这不是 CI —— 它依赖开发者本机的 agent CLI 和（真实模式下）provider 凭证。
# CI 侧的 hermetic 覆盖由 .github/workflows/harness-matrix.yml 负责；本脚本是
# 「一个阶段后要做的真实补充测试」。
#
# 退出码: 0 全绿 | 1 有 FAIL/TIMEOUT | 2 环境错误（CLI 缺失 / build 失败 / config 缺失）
# 产物:   ./.tmp/harness/<run-id>/{harness-summary.csv, harness-output/}
#         ./.tmp/harness/latest -> <run-id>
#
# 用法:
#   ./scripts/harness-baseline.sh                            # 只跑 mock 冒烟
#   ./scripts/harness-baseline.sh --real providers.yaml      # mock + 真实扫描
#   ./scripts/harness-baseline.sh --real providers.yaml --only-failing   # 只重跑红的
#   ./scripts/harness-baseline.sh --real providers.yaml --resume         # 续跑（跳已记录绿项）
#   ./scripts/harness-baseline.sh --extra "--timeout 5m"     # 透传任意 harness 参数

set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

REAL_CONFIG=""
ONLY_FAILING=0
RESUME=0
EXTRA_ARGS=""
HARNESS_BIN="${HARNESS_BIN:-./harness}"          # build 产物路径
OUT_ROOT="$ROOT/.tmp/harness"

usage() {
    sed -n '2,/^$/p' < "$0" | sed 's/^# \{0,1\}//'
    exit 2
}

# --- parse args ---
while [ $# -gt 0 ]; do
    case "$1" in
        --real)        REAL_CONFIG="$2"; shift 2 ;;
        --only-failing) ONLY_FAILING=1; shift ;;
        --resume)      RESUME=1; shift ;;
        --extra)       EXTRA_ARGS="$2"; shift 2 ;;
        -h|--help)     usage ;;
        *) echo "unknown arg: $1" >&2; usage ;;
    esac
done

# --- env preflight: agent CLIs on PATH ---
MISSING=""
for c in claude codex opencode; do
    if ! command -v "$c" >/dev/null 2>&1; then
        MISSING="$MISSING $c"
    fi
done
if [ -n "$MISSING" ]; then
    echo "✗ 环境错误：缺失 agent CLI:$MISSING" >&2
    echo "  请先安装对应 CLI 并加入 PATH。" >&2
    exit 2
fi

# --- build harness ---
echo "▶ go build -o $HARNESS_BIN ./cli/harness"
if ! go build -o "$HARNESS_BIN" ./cli/harness; then
    echo "✗ 环境错误：build 失败" >&2
    exit 2
fi

# --- run dir ---
RUN_ID="$(date +%Y%m%d-%H%M%S)"
RUN_DIR="$OUT_ROOT/$RUN_ID"
mkdir -p "$RUN_DIR"
# refresh the `latest` symlink (ln -nf is POSIX-safe on mac/linux)
ln -nfs "$RUN_ID" "$OUT_ROOT/latest"

SUMMARY="$RUN_DIR/harness-summary.csv"
OUTPUT_DIR="$RUN_DIR/harness-output"

# shared flags for every harness invocation
COMMON="--summary $SUMMARY --output-dir $OUTPUT_DIR"

# optional pass-throughs assembled once
OPTS=""
if [ "$ONLY_FAILING" = 1 ]; then OPTS="$OPTS --only-failing"; fi
if [ "$RESUME" = 1 ];        then OPTS="$OPTS --resume \"\""; fi
if [ -n "$EXTRA_ARGS" ];     then OPTS="$OPTS $EXTRA_ARGS"; fi

# harness wrapper: runs `agent batch ...`, returns its exit code without
# aborting the script (set -e), and prints a labelled header.
run_batch() {
    mode_label="$1"; shift
    echo
    echo "══ $mode_label ══"
    set +e
    # shellcheck disable=SC2086 # intentional word-splitting on OPTS/COMMON
    eval "\"$HARNESS_BIN\" agent batch $* $COMMON $OPTS"
    rc=$?
    set -e
    return $rc
}

OVERALL=0

# --- mock smoke (always) ---
if ! run_batch "mock 冒烟 (vmodel，零配置)" --mock; then
    OVERALL=1
fi

# --- optional real-provider sweep ---
if [ -n "$REAL_CONFIG" ]; then
    if [ ! -f "$REAL_CONFIG" ]; then
        echo "✗ 环境错误：config 文件不存在: $REAL_CONFIG" >&2
        exit 2
    fi
    # real sweep failures don't undo a green mock, but they count.
    if ! run_batch "真实 provider 扫描 ($REAL_CONFIG)" --config "$REAL_CONFIG"; then
        OVERALL=1
    fi
fi

# --- summary pointer ---
echo
echo "────────────────────────────────────────"
echo "产物目录: $RUN_DIR"
echo "  summary CSV: $SUMMARY"
echo "  逐条明细:   $OUTPUT_DIR/"
echo "  latest 软链: $OUT_ROOT/latest"
echo "────────────────────────────────────────"
if [ "$OVERALL" = 0 ]; then
    echo "✅ 基线全绿"
else
    echo "❌ 有失败/超时项（见上方表格与 summary CSV）"
    echo "   重跑红的: ./scripts/harness-baseline.sh --real $REAL_CONFIG --only-failing"
fi
exit "$OVERALL"
