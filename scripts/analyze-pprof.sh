#!/bin/bash
# =============================================================================
# pprof 性能数据分析脚本
# 用法: ./analyze-pprof.sh [pprof目录] [输出目录]
# =============================================================================

set -e

PPROF_DIR="${1:-./pprof}"
OUTPUT_DIR="${2:-./pprof-report}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# 检查依赖
check_dependencies() {
    info "检查依赖..."
    
    if ! command -v go &> /dev/null; then
        error "go 命令未找到，请安装 Go"
        exit 1
    fi
    
    if ! command -v graphviz &> /dev/null && ! command -v dot &> /dev/null; then
        warn "graphviz 未安装，将跳过图形生成"
        warn "安装方法: brew install graphviz (macOS) 或 apt-get install graphviz (Linux)"
        HAS_GRAPHVIZ=false
    else
        HAS_GRAPHVIZ=true
    fi
    
    success "依赖检查完成"
}

# 创建输出目录
setup_output_dir() {
    info "创建输出目录: $OUTPUT_DIR"
    mkdir -p "$OUTPUT_DIR"/{cpu,heap,goroutine,block,mutex,graphs,reports}
    success "输出目录创建完成"
}

# 获取最新的profile文件
get_latest_profile() {
    local profile_type=$1
    local dir="$PPROF_DIR/$profile_type"
    
    if [ -d "$dir" ]; then
        ls -t "$dir"/*.pprof 2>/dev/null | head -1
    fi
}

# 获取所有profile文件
get_all_profiles() {
    local profile_type=$1
    local dir="$PPROF_DIR/$profile_type"
    
    if [ -d "$dir" ]; then
        ls -t "$dir"/*.pprof 2>/dev/null
    fi
}

# =============================================================================
# CPU 分析
# =============================================================================
analyze_cpu() {
    info "========== CPU 性能分析 =========="
    
    local latest=$(get_latest_profile "cpu")
    if [ -z "$latest" ]; then
        warn "未找到 CPU profile 文件"
        return
    fi
    
    info "分析文件: $latest"
    local report="$OUTPUT_DIR/reports/cpu_analysis.txt"
    
    {
        echo "================================================================================"
        echo "                           CPU 性能分析报告"
        echo "================================================================================"
        echo "分析时间: $(date)"
        echo "Profile文件: $latest"
        echo ""
        
        echo "--------------------------------------------------------------------------------"
        echo "1. CPU 热点函数 (Top 30 - 按 flat 时间排序)"
        echo "--------------------------------------------------------------------------------"
        echo "说明: flat = 函数自身消耗的CPU时间, cum = 函数及其调用的所有函数消耗的总时间"
        echo ""
        go tool pprof -top -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "2. CPU 热点函数 (Top 30 - 按 cum 累积时间排序)"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 累积时间高的函数可能是性能瓶颈的入口点"
        echo ""
        go tool pprof -top -cum -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "3. 调用树分析"
        echo "--------------------------------------------------------------------------------"
        go tool pprof -tree -nodecount=50 "$latest" 2>/dev/null
        
    } > "$report"
    
    # 生成SVG调用图
    if [ "$HAS_GRAPHVIZ" = true ]; then
        info "生成 CPU 调用图..."
        go tool pprof -svg "$latest" > "$OUTPUT_DIR/graphs/cpu_callgraph.svg" 2>/dev/null || warn "SVG生成失败"
    fi
    
    success "CPU 分析完成: $report"
}

# =============================================================================
# 内存分析
# =============================================================================
analyze_heap() {
    info "========== 内存 (Heap) 分析 =========="
    
    local latest=$(get_latest_profile "heap")
    if [ -z "$latest" ]; then
        warn "未找到 Heap profile 文件"
        return
    fi
    
    info "分析文件: $latest"
    local report="$OUTPUT_DIR/reports/heap_analysis.txt"
    
    {
        echo "================================================================================"
        echo "                           内存 (Heap) 分析报告"
        echo "================================================================================"
        echo "分析时间: $(date)"
        echo "Profile文件: $latest"
        echo ""
        
        echo "--------------------------------------------------------------------------------"
        echo "1. 当前内存使用 (inuse_space) - Top 30"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 当前正在使用的内存，用于发现内存占用大户"
        echo ""
        go tool pprof -top -inuse_space -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "2. 当前对象数量 (inuse_objects) - Top 30"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 当前存活的对象数量，对象数量多可能导致GC压力"
        echo ""
        go tool pprof -top -inuse_objects -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "3. 累计内存分配 (alloc_space) - Top 30"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 程序运行期间累计分配的内存，用于发现频繁分配的热点"
        echo ""
        go tool pprof -top -alloc_space -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "4. 累计对象分配 (alloc_objects) - Top 30"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 累计分配的对象数量，频繁分配会增加GC负担"
        echo ""
        go tool pprof -top -alloc_objects -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "5. 内存分配调用树"
        echo "--------------------------------------------------------------------------------"
        go tool pprof -tree -inuse_space -nodecount=30 "$latest" 2>/dev/null
        
    } > "$report"
    
    # 生成SVG
    if [ "$HAS_GRAPHVIZ" = true ]; then
        info "生成内存调用图..."
        go tool pprof -svg -inuse_space "$latest" > "$OUTPUT_DIR/graphs/heap_inuse.svg" 2>/dev/null || warn "SVG生成失败"
        go tool pprof -svg -alloc_space "$latest" > "$OUTPUT_DIR/graphs/heap_alloc.svg" 2>/dev/null || warn "SVG生成失败"
    fi
    
    # 内存泄漏分析 - 比较多个时间点
    local all_profiles_str=$(get_all_profiles "heap")
    local all_profiles=($all_profiles_str)
    local profile_count=${#all_profiles[@]}
    if [ "$profile_count" -ge 2 ]; then
        info "进行内存泄漏分析 (比较多个时间点)..."
        local first="${all_profiles[$((profile_count-1))]}"  # 最早的
        local last="${all_profiles[0]}"                       # 最新的
        
        {
            echo ""
            echo "--------------------------------------------------------------------------------"
            echo "6. 内存泄漏分析 (对比早期和最新的heap profile)"
            echo "--------------------------------------------------------------------------------"
            echo "基准文件 (早期): $first"
            echo "对比文件 (最新): $last"
            echo ""
            echo "说明: 正值表示内存增长，负值表示内存减少"
            echo ""
            go tool pprof -top -diff_base="$first" -inuse_space "$last" 2>/dev/null || echo "对比分析失败"
        } >> "$report"
    fi
    
    success "内存分析完成: $report"
}

# =============================================================================
# Goroutine 分析
# =============================================================================
analyze_goroutine() {
    info "========== Goroutine 分析 =========="
    
    local latest=$(get_latest_profile "goroutine")
    if [ -z "$latest" ]; then
        warn "未找到 Goroutine profile 文件"
        return
    fi
    
    info "分析文件: $latest"
    local report="$OUTPUT_DIR/reports/goroutine_analysis.txt"
    
    {
        echo "================================================================================"
        echo "                           Goroutine 分析报告"
        echo "================================================================================"
        echo "分析时间: $(date)"
        echo "Profile文件: $latest"
        echo ""
        
        echo "--------------------------------------------------------------------------------"
        echo "1. Goroutine 分布 (Top 30)"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 显示goroutine堆积在哪些函数，用于发现goroutine泄漏"
        echo ""
        go tool pprof -top -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "2. Goroutine 调用树"
        echo "--------------------------------------------------------------------------------"
        go tool pprof -tree -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "3. 所有 Goroutine 堆栈 (前100个)"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 详细的goroutine堆栈信息"
        echo ""
        go tool pprof -traces "$latest" 2>/dev/null | head -500
        
    } > "$report"
    
    # 生成SVG
    if [ "$HAS_GRAPHVIZ" = true ]; then
        info "生成 Goroutine 调用图..."
        go tool pprof -svg "$latest" > "$OUTPUT_DIR/graphs/goroutine.svg" 2>/dev/null || warn "SVG生成失败"
    fi
    
    # Goroutine泄漏分析
    local all_profiles_str=$(get_all_profiles "goroutine")
    local all_profiles=($all_profiles_str)
    local profile_count=${#all_profiles[@]}
    if [ "$profile_count" -ge 2 ]; then
        info "进行 Goroutine 泄漏分析..."
        local first="${all_profiles[$((profile_count-1))]}"
        local last="${all_profiles[0]}"
        
        {
            echo ""
            echo "--------------------------------------------------------------------------------"
            echo "4. Goroutine 泄漏分析 (对比早期和最新)"
            echo "--------------------------------------------------------------------------------"
            echo "基准文件 (早期): $first"
            echo "对比文件 (最新): $last"
            echo ""
            go tool pprof -top -diff_base="$first" "$last" 2>/dev/null || echo "对比分析失败"
        } >> "$report"
    fi
    
    success "Goroutine 分析完成: $report"
}

# =============================================================================
# Block 分析
# =============================================================================
analyze_block() {
    info "========== Block (阻塞) 分析 =========="
    
    local latest=$(get_latest_profile "block")
    if [ -z "$latest" ]; then
        warn "未找到 Block profile 文件"
        return
    fi
    
    info "分析文件: $latest"
    local report="$OUTPUT_DIR/reports/block_analysis.txt"
    
    {
        echo "================================================================================"
        echo "                           Block (阻塞) 分析报告"
        echo "================================================================================"
        echo "分析时间: $(date)"
        echo "Profile文件: $latest"
        echo ""
        
        echo "--------------------------------------------------------------------------------"
        echo "1. 阻塞热点 (Top 30)"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 显示在哪些操作上发生了阻塞等待"
        echo ""
        go tool pprof -top -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "2. 阻塞调用树"
        echo "--------------------------------------------------------------------------------"
        go tool pprof -tree -nodecount=30 "$latest" 2>/dev/null
        
    } > "$report"
    
    if [ "$HAS_GRAPHVIZ" = true ]; then
        go tool pprof -svg "$latest" > "$OUTPUT_DIR/graphs/block.svg" 2>/dev/null || warn "SVG生成失败"
    fi
    
    success "Block 分析完成: $report"
}

# =============================================================================
# Mutex 分析
# =============================================================================
analyze_mutex() {
    info "========== Mutex (锁竞争) 分析 =========="
    
    local latest=$(get_latest_profile "mutex")
    if [ -z "$latest" ]; then
        warn "未找到 Mutex profile 文件"
        return
    fi
    
    info "分析文件: $latest"
    local report="$OUTPUT_DIR/reports/mutex_analysis.txt"
    
    {
        echo "================================================================================"
        echo "                           Mutex (锁竞争) 分析报告"
        echo "================================================================================"
        echo "分析时间: $(date)"
        echo "Profile文件: $latest"
        echo ""
        
        echo "--------------------------------------------------------------------------------"
        echo "1. 锁竞争热点 (Top 30)"
        echo "--------------------------------------------------------------------------------"
        echo "说明: 显示在哪些锁上发生了竞争等待"
        echo ""
        go tool pprof -top -nodecount=30 "$latest" 2>/dev/null
        
        echo ""
        echo "--------------------------------------------------------------------------------"
        echo "2. 锁竞争调用树"
        echo "--------------------------------------------------------------------------------"
        go tool pprof -tree -nodecount=30 "$latest" 2>/dev/null
        
    } > "$report"
    
    if [ "$HAS_GRAPHVIZ" = true ]; then
        go tool pprof -svg "$latest" > "$OUTPUT_DIR/graphs/mutex.svg" 2>/dev/null || warn "SVG生成失败"
    fi
    
    success "Mutex 分析完成: $report"
}

# =============================================================================
# 生成综合报告
# =============================================================================
generate_summary() {
    info "========== 生成综合报告 =========="
    
    local summary="$OUTPUT_DIR/SUMMARY.md"
    
    {
        echo "# pprof 性能分析综合报告"
        echo ""
        echo "**生成时间**: $(date)"
        echo ""
        echo "**分析目录**: $PPROF_DIR"
        echo ""
        echo "---"
        echo ""
        echo "## 📊 分析概览"
        echo ""
        echo "| Profile类型 | 文件数量 | 最新文件 |"
        echo "|------------|---------|---------|"
        
        for type in cpu heap goroutine block mutex allocs; do
            local dir="$PPROF_DIR/$type"
            if [ -d "$dir" ]; then
                local count=$(ls "$dir"/*.pprof 2>/dev/null | wc -l | tr -d ' ')
                local latest=$(ls -t "$dir"/*.pprof 2>/dev/null | head -1 | xargs basename 2>/dev/null || echo "N/A")
                echo "| $type | $count | $latest |"
            fi
        done
        
        echo ""
        echo "---"
        echo ""
        echo "## 🔥 CPU 分析要点"
        echo ""
        if [ -f "$OUTPUT_DIR/reports/cpu_analysis.txt" ]; then
            echo "详细报告: [cpu_analysis.txt](reports/cpu_analysis.txt)"
            echo ""
            echo "### Top 10 CPU 热点函数"
            echo '```'
            go tool pprof -top -nodecount=10 "$(get_latest_profile cpu)" 2>/dev/null || echo "无数据"
            echo '```'
        else
            echo "无 CPU profile 数据"
        fi
        
        echo ""
        echo "---"
        echo ""
        echo "## 💾 内存分析要点"
        echo ""
        if [ -f "$OUTPUT_DIR/reports/heap_analysis.txt" ]; then
            echo "详细报告: [heap_analysis.txt](reports/heap_analysis.txt)"
            echo ""
            echo "### Top 10 内存占用"
            echo '```'
            go tool pprof -top -inuse_space -nodecount=10 "$(get_latest_profile heap)" 2>/dev/null || echo "无数据"
            echo '```'
        else
            echo "无 Heap profile 数据"
        fi
        
        echo ""
        echo "---"
        echo ""
        echo "## 🔄 Goroutine 分析要点"
        echo ""
        if [ -f "$OUTPUT_DIR/reports/goroutine_analysis.txt" ]; then
            echo "详细报告: [goroutine_analysis.txt](reports/goroutine_analysis.txt)"
            echo ""
            echo "### Top 10 Goroutine 分布"
            echo '```'
            go tool pprof -top -nodecount=10 "$(get_latest_profile goroutine)" 2>/dev/null || echo "无数据"
            echo '```'
        else
            echo "无 Goroutine profile 数据"
        fi
        
        echo ""
        echo "---"
        echo ""
        echo "## 📁 生成的文件"
        echo ""
        echo "### 详细报告"
        echo ""
        for f in "$OUTPUT_DIR/reports"/*.txt; do
            [ -f "$f" ] && echo "- [$(basename $f)](reports/$(basename $f))"
        done
        
        echo ""
        echo "### 可视化图表"
        echo ""
        for f in "$OUTPUT_DIR/graphs"/*.svg; do
            [ -f "$f" ] && echo "- [$(basename $f)](graphs/$(basename $f))"
        done
        
        echo ""
        echo "---"
        echo ""
        echo "## 🛠 交互式分析命令"
        echo ""
        echo "### 启动 Web UI (推荐)"
        echo '```bash'
        echo "# CPU 分析"
        echo "go tool pprof -http=:8080 $(get_latest_profile cpu)"
        echo ""
        echo "# 内存分析"
        echo "go tool pprof -http=:8081 $(get_latest_profile heap)"
        echo ""
        echo "# Goroutine 分析"
        echo "go tool pprof -http=:8082 $(get_latest_profile goroutine)"
        echo '```'
        
        echo ""
        echo "### 命令行交互模式"
        echo '```bash'
        echo "go tool pprof $(get_latest_profile cpu)"
        echo ""
        echo "# 常用命令:"
        echo "# top        - 显示热点函数"
        echo "# top -cum   - 按累积时间排序"
        echo "# list func  - 查看函数源码"
        echo "# web        - 浏览器打开调用图"
        echo "# tree       - 树形展示"
        echo '```'
        
    } > "$summary"
    
    success "综合报告生成完成: $summary"
}

# =============================================================================
# 主函数
# =============================================================================
main() {
    echo ""
    echo "╔══════════════════════════════════════════════════════════════════════════════╗"
    echo "║                        pprof 性能数据分析工具                                 ║"
    echo "╚══════════════════════════════════════════════════════════════════════════════╝"
    echo ""
    
    # 检查pprof目录
    if [ ! -d "$PPROF_DIR" ]; then
        error "pprof 目录不存在: $PPROF_DIR"
        exit 1
    fi
    
    check_dependencies
    setup_output_dir
    
    echo ""
    
    # 执行各类分析
    analyze_cpu
    echo ""
    analyze_heap
    echo ""
    analyze_goroutine
    echo ""
    analyze_block
    echo ""
    analyze_mutex
    echo ""
    
    # 生成综合报告
    generate_summary
    
    echo ""
    echo "╔══════════════════════════════════════════════════════════════════════════════╗"
    echo "║                              分析完成!                                        ║"
    echo "╚══════════════════════════════════════════════════════════════════════════════╝"
    echo ""
    info "报告目录: $OUTPUT_DIR"
    info "综合报告: $OUTPUT_DIR/SUMMARY.md"
    echo ""
    info "快速查看 Web UI:"
    echo "  go tool pprof -http=:8080 $(get_latest_profile cpu)"
    echo ""
}

main "$@"
