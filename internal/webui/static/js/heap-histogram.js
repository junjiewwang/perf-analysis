/**
 * Heap Histogram Module
 * Class Histogram 表格模块：负责类直方图的展示和交互
 * 
 * 职责：
 * - 渲染 IDEA 风格的类直方图表格
 * - 处理排序、过滤、展开/折叠
 * - 管理包视图模式
 */

const HeapHistogram = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let currentData = [];
    let sortField = 'shallow';
    let sortAsc = false;
    let viewMode = 'flat'; // 'flat' | 'package'

    // ============================================
    // 私有方法
    // ============================================
    
    /**
     * 排序数据
     * @param {Array} data - 类数据
     * @param {string} field - 排序字段
     * @param {boolean} ascending - 是否升序
     * @returns {Array} 排序后的数据
     */
    function sortData(data, field, ascending) {
        return [...data].sort((a, b) => {
            let aVal, bVal;
            
            switch (field) {
                case 'name':
                    aVal = a.name.toLowerCase();
                    bVal = b.name.toLowerCase();
                    return ascending ? aVal.localeCompare(bVal) : bVal.localeCompare(aVal);
                case 'count':
                    aVal = a.instanceCount || 0;
                    bVal = b.instanceCount || 0;
                    break;
                case 'shallow':
                    aVal = a.size || 0;
                    bVal = b.size || 0;
                    break;
                case 'retained':
                    aVal = a.retained_size || 0;
                    bVal = b.retained_size || 0;
                    break;
                default:
                    aVal = a.size || 0;
                    bVal = b.size || 0;
            }
            
            return ascending ? aVal - bVal : bVal - aVal;
        });
    }

    /**
     * 生成表格行 HTML
     * @param {Object} cls - 类数据
     * @param {number} index - 索引
     * @param {number} maxShallow - 最大 shallow size
     * @param {number} maxRetained - 最大 retained size
     * @returns {string} HTML 字符串
     */
    function generateRowHtml(cls, index, maxShallow, maxRetained) {
        const businessRetainers = HeapCore.getState('businessRetainers');
        
        const hasRetainers = cls.retainers && cls.retainers.length > 0;
        const hasGCPaths = cls.gc_root_paths && cls.gc_root_paths.length > 0;
        const hasBusinessRetainers = businessRetainers[cls.name] && businessRetainers[cls.name].length > 0;
        const canExpand = hasRetainers || hasGCPaths || hasBusinessRetainers;
        
        // 计算进度条宽度
        const shallowBarWidth = maxShallow > 0 ? (cls.size / maxShallow) * 100 : 0;
        const retainedBarWidth = maxRetained > 0 ? ((cls.retained_size || 0) / maxRetained) * 100 : 0;
        
        // 格式化类名
        const formattedClassName = HeapCore.formatClassNameIDEA(cls.name);

        // 生成 retainer 展开区域
        let retainerSection = '';
        if (hasRetainers || hasBusinessRetainers) {
            const retainers = hasBusinessRetainers ? businessRetainers[cls.name] : cls.retainers;
            retainerSection = `
                <tr id="retainer-row-${index}" class="retainer-row" style="display: none;">
                    <td colspan="5">
                        <div class="retainer-tree">
                            ${retainers.slice(0, 10).map((r, ri) => `
                                <div class="retainer-tree-item" style="--depth: ${r.depth || 1}">
                                    <span class="tree-icon">${ri === 0 ? '└─' : '├─'}</span>
                                    <span class="retainer-class">
                                        ${Utils.escapeHtml(r.retainer_class || r.class_name)}
                                        ${r.field_name ? `<span class="retainer-field">.${Utils.escapeHtml(r.field_name)}</span>` : ''}
                                        ${r.is_gc_root ? `<span style="color: #4ade80; margin-left: 8px;">[GC ROOT: ${r.gc_root_type || 'ROOT'}]</span>` : ''}
                                    </span>
                                    <span class="retainer-stats">
                                        ${r.percentage ? `${r.percentage.toFixed(1)}%` : ''} · 
                                        ${Utils.formatNumber(r.retained_count || 0)} refs · 
                                        ${Utils.formatBytes(r.retained_size || 0)}
                                    </span>
                                </div>
                            `).join('')}
                            ${retainers.length > 10 ? `<div class="retainer-tree-item" style="--depth: 1; color: #808080;">... and ${retainers.length - 10} more</div>` : ''}
                        </div>
                    </td>
                </tr>
            `;
        }

        return `
            <tr id="class-row-${index}" class="${canExpand ? 'has-retainers' : ''}" ${canExpand ? `onclick="HeapHistogram.toggleRow(${index})"` : ''}>
                <td>
                    ${canExpand ? `<button class="expand-btn" id="expand-btn-${index}">▶</button>` : ''}
                </td>
                <td class="class-name">${formattedClassName}</td>
                <td class="instance-count">${Utils.formatNumber(cls.instanceCount)}</td>
                <td class="size-cell">
                    <div class="size-bar-bg" style="width: ${shallowBarWidth}%"></div>
                    <span class="size-value">${Utils.formatBytes(cls.size)}</span>
                </td>
                <td class="size-cell retained-cell">
                    <div class="size-bar-bg" style="width: ${retainedBarWidth}%"></div>
                    <span class="size-value">${cls.retained_size ? Utils.formatBytes(cls.retained_size) : '-'}</span>
                </td>
            </tr>
            ${retainerSection}
        `;
    }

    /**
     * 渲染平铺视图
     * @param {Array} data - 类数据
     */
    function renderFlatView(data) {
        const tbody = document.getElementById('heapClassTableBody');
        if (!tbody) return;

        const sortedData = sortData(data, sortField, sortAsc);
        
        const maxShallow = sortedData.length > 0 ? Math.max(...sortedData.map(c => c.size)) : 1;
        const maxRetained = sortedData.length > 0 ? Math.max(...sortedData.map(c => c.retained_size || 0)) : 1;

        tbody.innerHTML = sortedData.map((cls, i) => 
            generateRowHtml(cls, i, maxShallow, maxRetained)
        ).join('');
    }

    /**
     * 渲染包视图
     * @param {Array} data - 类数据
     */
    function renderPackageView(data) {
        const container = document.getElementById('heapPackageGroups');
        if (!container) return;

        const packageMap = HeapCore.groupByPackage(data);
        const packages = Array.from(packageMap.entries())
            .sort((a, b) => b[1].totalSize - a[1].totalSize);

        container.innerHTML = packages.map(([pkgName, pkg], idx) => {
            const classRows = pkg.classes.map((cls, i) => {
                const shortName = cls.name.split('.').pop();
                return `
                    <tr>
                        <td style="padding-left: 30px;">${i + 1}</td>
                        <td class="class-name" title="${Utils.escapeHtml(cls.name)}">${Utils.escapeHtml(shortName)}</td>
                        <td>${Utils.formatBytes(cls.size)}</td>
                        <td>${Utils.formatNumber(cls.instanceCount)}</td>
                        <td>${cls.percentage.toFixed(2)}%</td>
                    </tr>
                `;
            }).join('');

            return `
                <div class="heap-package-group">
                    <div class="heap-package-header" onclick="HeapHistogram.togglePackage(${idx})">
                        <span>📦 ${Utils.escapeHtml(pkgName)}</span>
                        <div class="heap-package-stats">
                            <span>Size: ${Utils.formatBytes(pkg.totalSize)}</span>
                            <span>Instances: ${Utils.formatNumber(pkg.totalInstances)}</span>
                            <span>Classes: ${pkg.classes.length}</span>
                        </div>
                    </div>
                    <div class="heap-package-content" id="pkg-content-${idx}">
                        <table class="heap-class-table">
                            <thead>
                                <tr>
                                    <th style="width: 50px">#</th>
                                    <th>Class</th>
                                    <th style="width: 120px">Size</th>
                                    <th style="width: 100px">Instances</th>
                                    <th style="width: 80px">%</th>
                                </tr>
                            </thead>
                            <tbody>${classRows}</tbody>
                        </table>
                    </div>
                </div>
            `;
        }).join('');
    }

    /**
     * 更新排序指示器
     */
    function updateSortIndicators() {
        document.querySelectorAll('.heap-class-table.idea-style th.sortable').forEach(th => {
            th.classList.remove('active');
            const field = th.dataset.sort;
            if (field === sortField) {
                th.classList.add('active');
                th.textContent = th.textContent.replace(/ [▲▼]$/, '') + (sortAsc ? ' ▲' : ' ▼');
            } else {
                th.textContent = th.textContent.replace(/ [▲▼]$/, '');
            }
        });
    }

    // ============================================
    // 公共方法
    // ============================================
    
    /**
     * 初始化模块
     */
    function init() {
        // 监听数据加载事件
        HeapCore.on('dataLoaded', function(data) {
            currentData = data.classData;
            render(currentData);
        });

        // 监听搜索事件
        HeapCore.on('searchChanged', function(searchTerm) {
            filter(searchTerm);
        });

        // 监听 retainer 数据更新
        HeapCore.on('retainerDataUpdated', function() {
            // 重新渲染以显示更新的 retainer 信息
            render(currentData);
        });
    }

    /**
     * 渲染直方图
     * @param {Array} data - 类数据
     */
    function render(data) {
        currentData = data || currentData;
        
        if (viewMode === 'flat') {
            renderFlatView(currentData);
        } else {
            renderPackageView(currentData);
        }
    }

    /**
     * 排序
     * @param {string} field - 排序字段
     */
    function sort(field) {
        if (sortField === field) {
            sortAsc = !sortAsc;
        } else {
            sortField = field;
            sortAsc = false;
        }
        
        updateSortIndicators();
        render(currentData);
    }

    /**
     * 过滤
     * @param {string} searchTerm - 搜索词
     */
    function filter(searchTerm) {
        const classData = HeapCore.getState('classData');
        
        if (!searchTerm) {
            currentData = classData;
        } else {
            const term = searchTerm.toLowerCase();
            currentData = classData.filter(cls => 
                cls.name.toLowerCase().includes(term)
            );
        }
        
        render(currentData);
    }

    /**
     * 清除搜索
     */
    function clearSearch() {
        const searchInput = document.getElementById('heapClassSearch');
        if (searchInput) {
            searchInput.value = '';
        }
        currentData = HeapCore.getState('classData');
        render(currentData);
    }

    /**
     * 设置视图模式
     * @param {string} mode - 'flat' | 'package'
     */
    function setViewMode(mode) {
        viewMode = mode;
        
        // 更新按钮状态
        document.getElementById('heapViewFlat')?.classList.toggle('active', mode === 'flat');
        document.getElementById('heapViewPackage')?.classList.toggle('active', mode === 'package');
        
        // 切换视图容器
        document.getElementById('heapFlatView').style.display = mode === 'flat' ? 'block' : 'none';
        document.getElementById('heapPackageView').style.display = mode === 'package' ? 'block' : 'none';

        render(currentData);
    }

    /**
     * 切换行展开/折叠
     * @param {number} idx - 行索引
     */
    function toggleRow(idx) {
        const retainerRow = document.getElementById(`retainer-row-${idx}`);
        const classRow = document.getElementById(`class-row-${idx}`);
        const expandBtn = document.getElementById(`expand-btn-${idx}`);
        
        if (retainerRow) {
            const isVisible = retainerRow.style.display !== 'none';
            retainerRow.style.display = isVisible ? 'none' : 'table-row';
            if (classRow) classRow.classList.toggle('expanded', !isVisible);
            if (expandBtn) expandBtn.textContent = isVisible ? '▶' : '▼';
        }
    }

    /**
     * 切换包展开/折叠
     * @param {number} idx - 包索引
     */
    function togglePackage(idx) {
        const content = document.getElementById(`pkg-content-${idx}`);
        if (content) {
            content.classList.toggle('expanded');
        }
    }

    /**
     * 搜索并定位到指定类
     * @param {string} className - 类名
     */
    function searchClass(className) {
        const searchInput = document.getElementById('heapClassSearch');
        if (!searchInput) return;

        const classData = HeapCore.getState('classData');
        
        // 尝试精确匹配
        let searchTerm = className;
        
        // 如果包含 $，可能是内部类
        if (className.includes('$')) {
            const mainClass = className.split('$')[0];
            const exactMatch = classData.find(c => c.name === className);
            if (!exactMatch) {
                const mainMatch = classData.find(c => c.name === mainClass);
                if (mainMatch) {
                    searchTerm = mainClass;
                } else {
                    searchTerm = className.split('.').pop();
                }
            }
        } else {
            const exactMatch = classData.find(c => c.name === className);
            if (!exactMatch) {
                searchTerm = className.split('.').pop();
            }
        }
        
        searchInput.value = searchTerm;
        filter(searchTerm);
        
        // 显示搜索结果通知
        const filtered = classData.filter(cls => 
            cls.name.toLowerCase().includes(searchTerm.toLowerCase())
        );
        
        if (filtered.length === 0) {
            HeapCore.showNotification(`未找到匹配 "${searchTerm}" 的类`, 'warning');
        } else {
            HeapCore.showNotification(`找到 ${filtered.length} 个匹配的类`, 'success');
        }
    }

    /**
     * 获取当前数据
     * @returns {Array} 当前显示的类数据
     */
    function getData() {
        return currentData;
    }

    // ============================================
    // 模块注册
    // ============================================
    
    const module = {
        init,
        render,
        sort,
        filter,
        clearSearch,
        setViewMode,
        toggleRow,
        togglePackage,
        searchClass,
        getData
    };

    // 自动注册到核心模块
    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('histogram', module);
    }

    return module;
})();

// 导出到全局
window.HeapHistogram = HeapHistogram;
