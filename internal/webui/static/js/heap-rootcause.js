/**
 * Heap Root Cause Module
 * Root Cause 分析模块：负责内存问题根因分析
 * 
 * 职责：
 * - 渲染快速诊断建议
 * - 渲染泄漏嫌疑类
 * - 渲染业务类分析
 * - 渲染集合类问题
 * - 加载详细 retainer 数据
 */

const HeapRootCause = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let summaryData = null;

    // ============================================
    // 私有方法
    // ============================================
    
    /**
     * 渲染快速诊断
     * @param {Object} diagnosis - 诊断数据
     */
    function renderQuickDiagnosis(diagnosis) {
        const container = document.getElementById('quickDiagnosisContainer');
        if (!container) return;

        const actionItems = diagnosis.action_items || [];
        if (actionItems.length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">✅</div>
                    <div>暂无诊断建议</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="action-items-list">
                ${actionItems.map((item, idx) => `
                    <div class="action-item priority-${item.priority}">
                        <div class="action-item-header">
                            <span class="action-priority">步骤 ${idx + 1}</span>
                            <span class="action-title">${Utils.escapeHtml(item.action)}</span>
                        </div>
                        <div class="action-detail">${Utils.escapeHtml(item.detail)}</div>
                        ${item.target ? `
                            <div class="action-buttons">
                                <button class="action-btn" onclick="HeapRootCause.searchClass('${Utils.escapeHtml(item.target).replace(/'/g, "\\'")}')">
                                    🔍 在 Class Histogram 中搜索
                                </button>
                                <button class="action-btn secondary" onclick="HeapRootCause.viewInRefGraph('${Utils.escapeHtml(item.target).replace(/'/g, "\\'")}')">
                                    🔗 查看引用图
                                </button>
                            </div>
                        ` : ''}
                    </div>
                `).join('')}
            </div>
        `;
    }

    /**
     * 渲染泄漏嫌疑
     * @param {Array} leakSuspects - 泄漏嫌疑数据
     * @param {Array} topClasses - 顶级类
     * @param {Array} suggestions - 建议
     */
    function renderLeakSuspects(leakSuspects, topClasses, suggestions) {
        const container = document.getElementById('leakSuspectsContainer');
        if (!container) return;

        // 如果没有预计算的泄漏嫌疑，从 topClasses 计算
        let suspects = leakSuspects;
        if (!suspects || suspects.length === 0) {
            suspects = [];
            for (const cls of topClasses.slice(0, 10)) {
                const relatedSuggestion = suggestions.find(s => 
                    s.func === cls.class_name || s.suggestion?.includes(cls.class_name)
                );
                
                let risk = 'low';
                let reasons = [];
                
                if (cls.percentage > 20) {
                    risk = 'high';
                    reasons.push(`占用堆内存 ${cls.percentage.toFixed(1)}%，超过 20% 阈值`);
                } else if (cls.percentage > 10) {
                    risk = 'medium';
                    reasons.push(`占用堆内存 ${cls.percentage.toFixed(1)}%，超过 10% 阈值`);
                } else if (cls.has_retainers && cls.instance_count > 10000) {
                    risk = 'medium';
                    reasons.push(`实例数量过多 (${Utils.formatNumber(cls.instance_count)})，可能存在集合类泄漏`);
                }

                if (relatedSuggestion) {
                    reasons.push(relatedSuggestion.suggestion);
                    if (cls.percentage > 10) risk = 'high';
                }

                if (reasons.length > 0) {
                    suspects.push({
                        class_name: cls.class_name,
                        risk_level: risk,
                        reasons: reasons,
                        total_size: cls.total_size,
                        instance_count: cls.instance_count,
                        percentage: cls.percentage
                    });
                }
            }
        }

        if (suspects.length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">✅</div>
                    <div>未发现明显的内存泄漏嫌疑</div>
                </div>
            `;
            return;
        }

        container.innerHTML = suspects.map(suspect => `
            <div class="leak-suspect-card ${suspect.risk_level}-risk">
                <div class="leak-suspect-header">
                    <div class="leak-suspect-class">${Utils.escapeHtml(suspect.class_name)}</div>
                    <span class="leak-suspect-risk ${suspect.risk_level}">
                        ${suspect.risk_level === 'high' ? '🔴 高风险' : suspect.risk_level === 'medium' ? '🟡 中风险' : '🟢 低风险'}
                    </span>
                </div>
                <div class="leak-suspect-stats">
                    <span>📊 ${(suspect.percentage || 0).toFixed(2)}%</span>
                    <span>💾 ${Utils.formatBytes(suspect.total_size || 0)}</span>
                    <span>📦 ${Utils.formatNumber(suspect.instance_count || 0)} 实例</span>
                </div>
                <div class="leak-suspect-reasons">
                    ${(suspect.reasons || []).map(r => `<div class="reason-item">💡 ${Utils.escapeHtml(r)}</div>`).join('')}
                </div>
                <div class="leak-suspect-actions">
                    <button class="action-btn small" onclick="HeapRootCause.searchClass('${Utils.escapeHtml(suspect.class_name).replace(/'/g, "\\'")}')">
                        🔍 搜索
                    </button>
                    <button class="action-btn small secondary" onclick="HeapRootCause.viewInRefGraph('${Utils.escapeHtml(suspect.class_name).replace(/'/g, "\\'")}')">
                        🔗 引用图
                    </button>
                </div>
            </div>
        `).join('');
    }

    /**
     * 渲染业务类
     * @param {Array} businessClasses - 业务类数据
     */
    function renderBusinessClasses(businessClasses) {
        const container = document.getElementById('businessClassesContainer');
        if (!container) return;

        if (!businessClasses || businessClasses.length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">📦</div>
                    <div>未发现业务类占用大量内存</div>
                    <div style="font-size: 12px; color: #666; margin-top: 5px;">
                        内存主要被 JDK/框架类占用，可能是正常情况
                    </div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="business-classes-list">
                <table class="business-classes-table">
                    <thead>
                        <tr>
                            <th>#</th>
                            <th>业务类名</th>
                            <th>大小</th>
                            <th>实例数</th>
                            <th>占比</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${businessClasses.map((cls, idx) => `
                            <tr>
                                <td>${idx + 1}</td>
                                <td class="class-name-cell" title="${Utils.escapeHtml(cls.class_name)}">
                                    ${Utils.escapeHtml(Utils.getShortClassName(cls.class_name))}
                                </td>
                                <td>${Utils.formatBytes(cls.total_size)}</td>
                                <td>${Utils.formatNumber(cls.instance_count)}</td>
                                <td>${(cls.percentage || 0).toFixed(2)}%</td>
                                <td>
                                    <button class="action-btn tiny" onclick="HeapRootCause.searchClass('${Utils.escapeHtml(cls.class_name).replace(/'/g, "\\'")}')">🔍</button>
                                </td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;
    }

    /**
     * 渲染集合类问题
     * @param {Array} issues - 集合类问题
     */
    function renderCollectionIssues(issues) {
        const container = document.getElementById('collectionIssuesContainer');
        if (!container) return;

        if (!issues || issues.length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">📋</div>
                    <div>未发现集合类异常</div>
                </div>
            `;
            return;
        }

        container.innerHTML = issues.map(issue => `
            <div class="collection-issue-card">
                <div class="issue-header">
                    <span class="issue-class">${Utils.escapeHtml(issue.class_name)}</span>
                    <span class="issue-count">${Utils.formatNumber(issue.instance_count)} 实例</span>
                </div>
                <div class="issue-detail">
                    <span>💾 ${Utils.formatBytes(issue.total_size)}</span>
                    <span class="issue-desc">⚠️ ${Utils.escapeHtml(issue.issue)}</span>
                </div>
            </div>
        `).join('');
    }

    /**
     * 渲染建议
     * @param {Array} suggestions - 建议数据
     */
    function renderSuggestions(suggestions) {
        const container = document.getElementById('heapSuggestionsContainer');
        if (!container) return;

        if (!suggestions || suggestions.length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">📝</div>
                    <div>暂无优化建议</div>
                </div>
            `;
            return;
        }

        container.innerHTML = suggestions.map(sug => `
            <div class="suggestion-card">
                <span class="suggestion-icon">💡</span>
                <div class="suggestion-content">
                    <span class="suggestion-text">${Utils.escapeHtml(sug.suggestion)}</span>
                    ${sug.func ? `
                        <div class="suggestion-func">
                            📍 ${Utils.escapeHtml(sug.func)}
                            <button class="action-btn tiny" onclick="HeapRootCause.searchClass('${Utils.escapeHtml(sug.func).replace(/'/g, "\\'")}')">🔍</button>
                        </div>
                    ` : ''}
                </div>
            </div>
        `).join('');
    }

    /**
     * 渲染业务 Retainers
     * @param {Object} retainers - retainer 数据
     */
    function renderBusinessRetainers(retainers) {
        const container = document.getElementById('businessRetainersContainer');
        if (!container) return;

        if (!retainers || Object.keys(retainers).length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">📊</div>
                    <div>未找到业务级别的 Retainer 数据</div>
                </div>
            `;
            return;
        }

        // 按 retained size 排序
        const sortedEntries = Object.entries(retainers)
            .map(([className, items]) => ({
                className,
                items,
                totalRetained: items.reduce((sum, r) => sum + r.retained_size, 0),
                hasGCRoots: items.some(r => r.is_gc_root)
            }))
            .sort((a, b) => b.totalRetained - a.totalRetained);

        container.innerHTML = sortedEntries.map((entry, idx) => `
            <div class="business-retainer-group">
                <div class="business-retainer-header" onclick="HeapRootCause.toggleBusinessGroup(${idx})">
                    <div class="business-retainer-target">
                        🎯 ${Utils.escapeHtml(entry.className)}
                        ${entry.hasGCRoots ? '<span class="gc-root-badge">Contains GC Roots</span>' : ''}
                    </div>
                    <div class="business-retainer-summary">
                        <span>📦 ${entry.items.length} retainers</span>
                        <span>💾 ${Utils.formatBytes(entry.totalRetained)}</span>
                    </div>
                </div>
                <div class="business-retainer-content" id="business-group-${idx}">
                    ${entry.items.map(r => `
                        <div class="business-retainer-item ${r.is_gc_root ? 'gc-root' : ''}">
                            <div style="flex: 1;">
                                <div class="business-retainer-class">
                                    <span class="depth-indicator">${r.depth}</span>
                                    ${Utils.escapeHtml(r.class_name)}
                                    ${r.is_gc_root ? `<span class="gc-root-badge">${r.gc_root_type || 'GC ROOT'}</span>` : ''}
                                </div>
                                ${r.field_path && r.field_path.length > 0 ? 
                                    `<div class="business-retainer-path">via ${r.field_path.join(' → ')}</div>` : ''}
                            </div>
                            <div class="business-retainer-metrics">
                                <span>📊 ${r.percentage.toFixed(1)}%</span>
                                <span>📦 ${Utils.formatNumber(r.retained_count)} refs</span>
                                <span>💾 ${Utils.formatBytes(r.retained_size)}</span>
                            </div>
                        </div>
                    `).join('')}
                </div>
            </div>
        `).join('');
    }

    // ============================================
    // 公共方法
    // ============================================
    
    /**
     * 初始化模块
     */
    function init() {
        // 无需特殊初始化
    }

    /**
     * 渲染根因分析
     * @param {Object} data - 摘要数据
     */
    function render(data) {
        summaryData = data;
        
        const suggestions = data.suggestions || [];
        const topClasses = data.data?.top_classes || [];
        const quickDiagnosis = data.data?.quick_diagnosis || {};

        // 渲染各个部分
        renderQuickDiagnosis(quickDiagnosis);
        renderLeakSuspects(quickDiagnosis.leak_suspects || [], topClasses, suggestions);
        renderBusinessClasses(quickDiagnosis.top_business_classes || []);
        renderCollectionIssues(quickDiagnosis.collection_issues || []);
        renderSuggestions(suggestions);

        // 按需加载详细 retainer 数据
        loadDetailedRetainers();
    }

    /**
     * 加载详细 Retainer 数据
     */
    async function loadDetailedRetainers() {
        const container = document.getElementById('businessRetainersContainer');
        if (!container) return;

        container.innerHTML = '<div class="loading">加载详细 Retainer 数据中...</div>';

        try {
            const taskId = new URLSearchParams(window.location.search).get('task') || '';
            const response = await fetch(`/api/retainers?task=${taskId}`);
            
            if (!response.ok) {
                throw new Error('Failed to load retainer data');
            }

            const data = await response.json();
            
            // 更新核心状态
            HeapCore.updateRetainerData(data);

            renderBusinessRetainers(data.business_retainers || {});
        } catch (error) {
            console.error('Failed to load detailed retainers:', error);
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">⚠️</div>
                    <div>无法加载详细的 Retainer 数据</div>
                    <div style="font-size: 12px; margin-top: 5px;">${error.message}</div>
                </div>
            `;
        }
    }

    /**
     * 切换业务组展开/折叠
     * @param {number} idx - 组索引
     */
    function toggleBusinessGroup(idx) {
        const content = document.getElementById(`business-group-${idx}`);
        if (content) {
            content.classList.toggle('expanded');
        }
    }

    /**
     * 过滤根因
     */
    function filter() {
        const searchTerm = document.getElementById('rootCauseSearch')?.value?.toLowerCase() || '';
        const businessRetainers = HeapCore.getState('businessRetainers');
        
        if (!searchTerm) {
            renderBusinessRetainers(businessRetainers);
            return;
        }

        const filtered = {};
        for (const [className, items] of Object.entries(businessRetainers)) {
            if (className.toLowerCase().includes(searchTerm)) {
                filtered[className] = items;
            } else {
                const matchingItems = items.filter(r => 
                    r.class_name.toLowerCase().includes(searchTerm)
                );
                if (matchingItems.length > 0) {
                    filtered[className] = matchingItems;
                }
            }
        }

        renderBusinessRetainers(filtered);
    }

    /**
     * 搜索类（跳转到 Histogram）
     * @param {string} className - 类名
     */
    function searchClass(className) {
        if (typeof App !== 'undefined') {
            App.showPanel('heaphistogram');
        }
        HeapHistogram.searchClass(className);
    }

    /**
     * 在引用图中查看
     * @param {string} className - 类名
     */
    function viewInRefGraph(className) {
        if (typeof App !== 'undefined') {
            App.showPanel('heaprefgraph');
        }
        HeapRefGraph.viewClass(className);
    }

    // ============================================
    // 模块注册
    // ============================================
    
    const module = {
        init,
        render,
        loadDetailedRetainers,
        toggleBusinessGroup,
        filter,
        searchClass,
        viewInRefGraph
    };

    // 自动注册到核心模块
    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('rootcause', module);
    }

    return module;
})();

// 导出到全局
window.HeapRootCause = HeapRootCause;
