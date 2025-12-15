/**
 * Heap Histogram Module
 * Class Histogram 表格模块：负责类直方图的展示和交互
 * 
 * 职责：
 * - 渲染类直方图表格（平铺显示，无展开）
 * - 处理排序、过滤、分页
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
    let currentPage = 1;
    let pageSize = 100;
    let totalPages = 1;

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
     * 生成表格行 HTML（简化版，无展开功能）
     * @param {Object} cls - 类数据
     * @param {number} index - 索引
     * @param {number} maxShallow - 最大 shallow size
     * @param {number} maxRetained - 最大 retained size
     * @param {number} globalIndex - 全局索引（用于显示序号）
     * @returns {string} HTML 字符串
     */
    function generateRowHtml(cls, index, maxShallow, maxRetained, globalIndex) {
        // 计算进度条宽度
        const shallowBarWidth = maxShallow > 0 ? (cls.size / maxShallow) * 100 : 0;
        const retainedBarWidth = maxRetained > 0 ? ((cls.retained_size || 0) / maxRetained) * 100 : 0;
        
        // 格式化类名（IDEA 风格：包名灰色，类名高亮）
        const formattedClassName = formatClassNameSimple(cls.name);

        return `
            <tr class="hover:bg-gray-800/50 transition-colors">
                <td class="text-center text-gray-500 text-xs w-12">${globalIndex}</td>
                <td class="class-name font-mono text-sm">${formattedClassName}</td>
                <td class="text-right text-gray-300 tabular-nums">${Utils.formatNumber(cls.instanceCount)}</td>
                <td class="size-cell relative">
                    <div class="size-bar-bg" style="width: ${shallowBarWidth}%"></div>
                    <span class="size-value">${Utils.formatBytes(cls.size)}</span>
                </td>
                <td class="size-cell retained-cell relative">
                    <div class="size-bar-bg" style="width: ${retainedBarWidth}%"></div>
                    <span class="size-value">${cls.retained_size ? Utils.formatBytes(cls.retained_size) : '-'}</span>
                </td>
            </tr>
        `;
    }

    /**
     * 简化的类名格式化
     * @param {string} className - 完整类名
     * @returns {string} 格式化后的 HTML
     */
    function formatClassNameSimple(className) {
        if (!className) return '';
        
        // 处理数组类型
        if (className.endsWith('[]')) {
            const baseType = className.slice(0, -2);
            const formatted = formatClassNameSimple(baseType);
            return formatted + '<span class="text-gray-400">[]</span>';
        }
        
        const lastDot = className.lastIndexOf('.');
        if (lastDot === -1) {
            // 没有包名，直接返回高亮的类名
            return `<span class="text-yellow-400 font-semibold">${Utils.escapeHtml(className)}</span>`;
        }
        
        const packageName = className.substring(0, lastDot + 1);
        const simpleName = className.substring(lastDot + 1);
        
        return `<span class="text-green-600">${Utils.escapeHtml(packageName)}</span><span class="text-yellow-400 font-semibold">${Utils.escapeHtml(simpleName)}</span>`;
    }

    /**
     * 渲染分页控件
     * @param {number} total - 总数据量
     */
    function renderPagination(total) {
        totalPages = Math.ceil(total / pageSize);
        const container = document.getElementById('heapPagination');
        if (!container) return;

        if (totalPages <= 1) {
            container.innerHTML = '';
            return;
        }

        const startItem = (currentPage - 1) * pageSize + 1;
        const endItem = Math.min(currentPage * pageSize, total);

        container.innerHTML = `
            <div class="flex items-center justify-between py-3 px-4 bg-gray-800 rounded-lg mt-4">
                <div class="text-sm text-gray-400">
                    显示 <span class="text-white font-medium">${startItem}-${endItem}</span> 
                    共 <span class="text-white font-medium">${Utils.formatNumber(total)}</span> 个类
                </div>
                <div class="flex items-center gap-2">
                    <button onclick="HeapHistogram.goToPage(1)" 
                        class="px-3 py-1.5 rounded text-sm ${currentPage === 1 ? 'bg-gray-700 text-gray-500 cursor-not-allowed' : 'bg-gray-700 text-gray-300 hover:bg-gray-600'}"
                        ${currentPage === 1 ? 'disabled' : ''}>
                        首页
                    </button>
                    <button onclick="HeapHistogram.goToPage(${currentPage - 1})" 
                        class="px-3 py-1.5 rounded text-sm ${currentPage === 1 ? 'bg-gray-700 text-gray-500 cursor-not-allowed' : 'bg-gray-700 text-gray-300 hover:bg-gray-600'}"
                        ${currentPage === 1 ? 'disabled' : ''}>
                        上一页
                    </button>
                    <span class="px-3 py-1.5 text-sm text-gray-300">
                        第 <span class="text-white font-medium">${currentPage}</span> / ${totalPages} 页
                    </span>
                    <button onclick="HeapHistogram.goToPage(${currentPage + 1})" 
                        class="px-3 py-1.5 rounded text-sm ${currentPage === totalPages ? 'bg-gray-700 text-gray-500 cursor-not-allowed' : 'bg-gray-700 text-gray-300 hover:bg-gray-600'}"
                        ${currentPage === totalPages ? 'disabled' : ''}>
                        下一页
                    </button>
                    <button onclick="HeapHistogram.goToPage(${totalPages})" 
                        class="px-3 py-1.5 rounded text-sm ${currentPage === totalPages ? 'bg-gray-700 text-gray-500 cursor-not-allowed' : 'bg-gray-700 text-gray-300 hover:bg-gray-600'}"
                        ${currentPage === totalPages ? 'disabled' : ''}>
                        末页
                    </button>
                    <select onchange="HeapHistogram.setPageSize(this.value)" 
                        class="ml-4 px-2 py-1.5 bg-gray-700 text-gray-300 rounded text-sm border border-gray-600">
                        <option value="50" ${pageSize === 50 ? 'selected' : ''}>50条/页</option>
                        <option value="100" ${pageSize === 100 ? 'selected' : ''}>100条/页</option>
                        <option value="200" ${pageSize === 200 ? 'selected' : ''}>200条/页</option>
                        <option value="500" ${pageSize === 500 ? 'selected' : ''}>500条/页</option>
                        <option value="-1" ${pageSize === -1 ? 'selected' : ''}>全部</option>
                    </select>
                </div>
            </div>
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
        
        // 分页处理
        let displayData;
        if (pageSize === -1) {
            displayData = sortedData;
        } else {
            const startIdx = (currentPage - 1) * pageSize;
            displayData = sortedData.slice(startIdx, startIdx + pageSize);
        }
        
        const maxShallow = sortedData.length > 0 ? Math.max(...sortedData.map(c => c.size)) : 1;
        const maxRetained = sortedData.length > 0 ? Math.max(...sortedData.map(c => c.retained_size || 0)) : 1;

        const startIndex = pageSize === -1 ? 0 : (currentPage - 1) * pageSize;
        
        tbody.innerHTML = displayData.map((cls, i) => 
            generateRowHtml(cls, i, maxShallow, maxRetained, startIndex + i + 1)
        ).join('');

        // 渲染分页
        renderPagination(sortedData.length);
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
                    <tr class="hover:bg-gray-700/50">
                        <td class="text-center text-gray-500 text-xs pl-8">${i + 1}</td>
                        <td class="font-mono text-sm text-yellow-400" title="${Utils.escapeHtml(cls.name)}">${Utils.escapeHtml(shortName)}</td>
                        <td class="text-right text-gray-300">${Utils.formatBytes(cls.size)}</td>
                        <td class="text-right text-gray-300">${Utils.formatNumber(cls.instanceCount)}</td>
                        <td class="text-right text-gray-400">${cls.percentage.toFixed(2)}%</td>
                    </tr>
                `;
            }).join('');

            return `
                <div class="mb-3 bg-gray-800 rounded-lg overflow-hidden">
                    <div class="flex justify-between items-center px-4 py-3 bg-gray-700 cursor-pointer hover:bg-gray-600 transition-colors" onclick="HeapHistogram.togglePackage(${idx})">
                        <span class="font-medium text-gray-200">
                            <span class="text-lg mr-2">📦</span>
                            ${Utils.escapeHtml(pkgName)}
                            <span class="text-gray-400 text-sm ml-2">(${pkg.classes.length} classes)</span>
                        </span>
                        <div class="flex gap-6 text-sm text-gray-400">
                            <span>Size: <span class="text-blue-400 font-medium">${Utils.formatBytes(pkg.totalSize)}</span></span>
                            <span>Instances: <span class="text-green-400 font-medium">${Utils.formatNumber(pkg.totalInstances)}</span></span>
                        </div>
                    </div>
                    <div class="hidden" id="pkg-content-${idx}">
                        <table class="w-full">
                            <thead>
                                <tr class="bg-gray-750 text-gray-400 text-xs uppercase">
                                    <th class="py-2 px-4 text-left w-12">#</th>
                                    <th class="py-2 px-4 text-left">Class</th>
                                    <th class="py-2 px-4 text-right w-28">Size</th>
                                    <th class="py-2 px-4 text-right w-24">Instances</th>
                                    <th class="py-2 px-4 text-right w-20">%</th>
                                </tr>
                            </thead>
                            <tbody class="text-sm">${classRows}</tbody>
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
        document.querySelectorAll('#heapHistogramTable th.sortable').forEach(th => {
            th.classList.remove('active');
            const field = th.dataset.sort;
            const arrow = th.querySelector('.sort-arrow');
            if (field === sortField) {
                th.classList.add('active');
                if (arrow) arrow.textContent = sortAsc ? '▲' : '▼';
            } else {
                if (arrow) arrow.textContent = '';
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
            currentPage = 1;
            render(currentData);
        });

        // 监听搜索事件
        HeapCore.on('searchChanged', function(searchTerm) {
            currentPage = 1;
            filter(searchTerm);
        });

        // 监听 retainer 数据更新
        HeapCore.on('retainerDataUpdated', function() {
            // 重新渲染
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
        
        currentPage = 1;
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
        
        currentPage = 1;
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
        currentPage = 1;
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
     * 切换包展开/折叠
     * @param {number} idx - 包索引
     */
    function togglePackage(idx) {
        const content = document.getElementById(`pkg-content-${idx}`);
        if (content) {
            content.classList.toggle('hidden');
        }
    }

    /**
     * 跳转到指定页
     * @param {number} page - 页码
     */
    function goToPage(page) {
        if (page < 1 || page > totalPages) return;
        currentPage = page;
        render(currentData);
        
        // 滚动到表格顶部
        const container = document.getElementById('heapHistogramContainer');
        if (container) {
            container.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }
    }

    /**
     * 设置每页显示数量
     * @param {string|number} size - 每页数量
     */
    function setPageSize(size) {
        pageSize = parseInt(size);
        currentPage = 1;
        render(currentData);
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
        togglePackage,
        goToPage,
        setPageSize,
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
