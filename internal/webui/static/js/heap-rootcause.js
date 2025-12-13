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
 * - 提供具体可执行的优化建议
 */

const HeapRootCause = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let summaryData = null;
    let diagnosisResult = null;

    // ============================================
    // 智能诊断引擎
    // ============================================

    /**
     * 执行智能诊断
     */
    function runSmartDiagnosis(data) {
        const topClasses = data.data?.top_classes || [];
        const totalHeapSize = data.data?.total_heap_size || 0;
        const suggestions = data.suggestions || [];
        
        const result = {
            actionItems: [],
            leakSuspects: [],
            businessClasses: [],
            collectionIssues: []
        };

        // 分析 Top 类
        for (let i = 0; i < Math.min(topClasses.length, 20); i++) {
            const cls = topClasses[i];
            const className = cls.class_name || '';
            const percentage = cls.percentage || 0;
            const instanceCount = cls.instance_count || 0;
            const retainers = cls.retainers || [];

            // 检测泄漏嫌疑
            const leakInfo = analyzeLeakSuspect(cls, retainers);
            if (leakInfo) {
                result.leakSuspects.push(leakInfo);
            }

            // 检测集合问题
            const collectionInfo = analyzeCollectionIssue(cls);
            if (collectionInfo) {
                result.collectionIssues.push(collectionInfo);
            }

            // 检测业务类
            if (isBusinessClass(className) && percentage > 3) {
                result.businessClasses.push({
                    class_name: className,
                    total_size: cls.total_size,
                    instance_count: instanceCount,
                    percentage: percentage,
                    retainers: retainers.slice(0, 3)
                });
            }
        }

        // 生成具体的行动项
        result.actionItems = generateActionItems(result, topClasses, totalHeapSize);

        return result;
    }

    /**
     * 分析泄漏嫌疑
     */
    function analyzeLeakSuspect(cls, retainers) {
        const className = cls.class_name || '';
        const percentage = cls.percentage || 0;
        const instanceCount = cls.instance_count || 0;

        let risk = 'low';
        const reasons = [];
        const solutions = [];

        // 规则 1: 高内存占用
        if (percentage > 20) {
            risk = 'high';
            reasons.push(`占用 ${percentage.toFixed(1)}% 堆内存，远超正常水平`);
            solutions.push('检查是否有缓存未清理或数据累积');
        } else if (percentage > 10) {
            risk = risk === 'high' ? 'high' : 'medium';
            reasons.push(`占用 ${percentage.toFixed(1)}% 堆内存`);
        }

        // 规则 2: 检查 retainer 模式
        if (retainers.length > 0) {
            const topRetainer = retainers[0];
            const retainerClass = topRetainer.retainer_class || '';
            const fieldName = topRetainer.field_name || '';

            // 静态字段持有
            if (topRetainer.depth === 1 || fieldName.includes('static')) {
                risk = 'high';
                reasons.push(`被静态字段持有: ${getShortClassName(retainerClass)}.${fieldName}`);
                solutions.push('静态字段持有的对象生命周期与应用相同，考虑使用 WeakReference 或添加清理机制');
            }

            // 缓存持有
            if (retainerClass.toLowerCase().includes('cache') || 
                fieldName.toLowerCase().includes('cache')) {
                risk = risk === 'low' ? 'medium' : risk;
                reasons.push(`被缓存持有: ${getShortClassName(retainerClass)}`);
                solutions.push('检查缓存是否有过期策略，考虑使用 LRU 或添加大小限制');
            }

            // 集合持有
            if (retainerClass.includes('Map') || retainerClass.includes('List')) {
                reasons.push(`被集合持有: ${getShortClassName(retainerClass)}`);
                solutions.push('检查集合是否在使用后被正确清理');
            }
        }

        // 规则 3: 实例数异常
        if (instanceCount > 100000) {
            risk = risk === 'low' ? 'medium' : risk;
            reasons.push(`实例数量异常: ${Utils.formatNumber(instanceCount)}`);
            solutions.push('检查是否有对象创建过多或未释放的问题');
        }

        if (reasons.length === 0) return null;

        return {
            class_name: className,
            risk_level: risk,
            reasons: reasons,
            solutions: solutions,
            total_size: cls.total_size,
            instance_count: instanceCount,
            percentage: percentage,
            retainers: retainers.slice(0, 3)
        };
    }

    /**
     * 分析集合问题
     */
    function analyzeCollectionIssue(cls) {
        const className = cls.class_name || '';
        const instanceCount = cls.instance_count || 0;

        const collectionTypes = {
            'java.util.HashMap': { threshold: 10000, issue: 'HashMap 实例过多' },
            'java.util.ArrayList': { threshold: 10000, issue: 'ArrayList 实例过多' },
            'java.util.LinkedList': { threshold: 5000, issue: 'LinkedList 实例过多，考虑使用 ArrayList' },
            'java.util.HashSet': { threshold: 10000, issue: 'HashSet 实例过多' },
            'java.util.concurrent.ConcurrentHashMap': { threshold: 5000, issue: 'ConcurrentHashMap 实例过多' },
            'java.util.LinkedHashMap': { threshold: 5000, issue: 'LinkedHashMap 实例过多' }
        };

        for (const [type, config] of Object.entries(collectionTypes)) {
            if (className.includes(type.split('.').pop()) && instanceCount > config.threshold) {
                return {
                    class_name: className,
                    instance_count: instanceCount,
                    total_size: cls.total_size,
                    issue: config.issue,
                    suggestion: `当前有 ${Utils.formatNumber(instanceCount)} 个实例，检查是否在循环中创建或缓存未清理`
                };
            }
        }

        return null;
    }

    /**
     * 生成具体行动项
     */
    function generateActionItems(result, topClasses, totalHeapSize) {
        const items = [];
        let priority = 1;

        // 高风险泄漏嫌疑
        const highRiskLeaks = result.leakSuspects.filter(s => s.risk_level === 'high');
        if (highRiskLeaks.length > 0) {
            const topLeak = highRiskLeaks[0];
            items.push({
                priority: priority++,
                action: `检查 ${getShortClassName(topLeak.class_name)}`,
                detail: `${topLeak.reasons[0]}。${topLeak.solutions[0] || ''}`,
                target: topLeak.class_name,
                severity: 'critical'
            });
        }

        // 集合问题
        if (result.collectionIssues.length > 0) {
            const topIssue = result.collectionIssues[0];
            items.push({
                priority: priority++,
                action: `优化 ${getShortClassName(topIssue.class_name)} 使用`,
                detail: topIssue.suggestion,
                target: topIssue.class_name,
                severity: 'warning'
            });
        }

        // 业务类占用
        if (result.businessClasses.length > 0) {
            const topBusiness = result.businessClasses[0];
            if (topBusiness.percentage > 10) {
                items.push({
                    priority: priority++,
                    action: `分析业务类 ${getShortClassName(topBusiness.class_name)}`,
                    detail: `业务类占用 ${topBusiness.percentage.toFixed(1)}% 内存，检查数据结构是否合理`,
                    target: topBusiness.class_name,
                    severity: 'info'
                });
            }
        }

        // byte[] 问题
        const byteArrayClass = topClasses.find(c => (c.class_name || '').includes('byte[]'));
        if (byteArrayClass && byteArrayClass.percentage > 20) {
            items.push({
                priority: priority++,
                action: '检查 byte[] 缓冲区',
                detail: `byte[] 占用 ${byteArrayClass.percentage.toFixed(1)}% 内存，检查 I/O 流是否正确关闭`,
                target: 'byte[]',
                severity: 'warning'
            });
        }

        // String 问题
        const stringClass = topClasses.find(c => 
            (c.class_name || '') === 'java.lang.String' || (c.class_name || '') === 'String'
        );
        if (stringClass && stringClass.instance_count > 500000) {
            items.push({
                priority: priority++,
                action: '优化 String 使用',
                detail: `${Utils.formatNumber(stringClass.instance_count)} 个 String 对象，使用 StringBuilder 替代字符串拼接`,
                target: 'java.lang.String',
                severity: 'info'
            });
        }

        return items;
    }

    function getShortClassName(fullName) {
        if (!fullName) return '';
        const lastDot = fullName.lastIndexOf('.');
        return lastDot === -1 ? fullName : fullName.substring(lastDot + 1);
    }

    function isBusinessClass(className) {
        if (!className) return false;
        return !className.startsWith('java.') && 
               !className.startsWith('javax.') &&
               !className.startsWith('sun.') && 
               !className.startsWith('jdk.') &&
               !className.startsWith('com.sun.') &&
               !className.startsWith('org.springframework.') &&
               !className.startsWith('org.apache.') &&
               !className.startsWith('io.netty.') &&
               !className.startsWith('com.google.') &&
               !className.includes('[]');
    }

    // ============================================
    // 渲染方法
    // ============================================
    
    /**
     * 渲染快速诊断
     * @param {Object} diagnosis - 诊断数据
     */
    function renderQuickDiagnosis(diagnosis) {
        const container = document.getElementById('quickDiagnosisContainer');
        if (!container) return;

        // 优先使用智能诊断结果
        const actionItems = diagnosisResult?.actionItems || diagnosis.action_items || [];
        
        if (actionItems.length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">✅</div>
                    <div>未检测到明显问题</div>
                    <div style="font-size: 12px; color: #666; margin-top: 5px;">堆内存使用正常</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="action-items-list">
                ${actionItems.map((item, idx) => `
                    <div class="action-item ${item.severity || 'info'}">
                        <div class="action-item-header">
                            <span class="action-priority ${item.severity || 'info'}">
                                ${getSeverityIcon(item.severity)} 步骤 ${idx + 1}
                            </span>
                            <span class="action-title">${Utils.escapeHtml(item.action)}</span>
                        </div>
                        <div class="action-detail">${Utils.escapeHtml(item.detail)}</div>
                        ${item.target ? `
                            <div class="action-buttons">
                                <button class="action-btn" onclick="HeapRootCause.searchClass('${Utils.escapeHtml(item.target).replace(/'/g, "\\'")}')">
                                    🔍 在 Histogram 中搜索
                                </button>
                                <button class="action-btn secondary" onclick="HeapRootCause.viewRetainers('${Utils.escapeHtml(item.target).replace(/'/g, "\\'")}')">
                                    🔗 查看持有者
                                </button>
                            </div>
                        ` : ''}
                    </div>
                `).join('')}
            </div>
        `;
    }

    function getSeverityIcon(severity) {
        const icons = { critical: '🔴', warning: '🟡', info: '🔵' };
        return icons[severity] || '🔵';
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
        
        // 运行智能诊断
        diagnosisResult = runSmartDiagnosis(data);
        
        const suggestions = data.suggestions || [];
        const topClasses = data.data?.top_classes || [];
        const quickDiagnosis = data.data?.quick_diagnosis || {};

        // 渲染各个部分 - 优先使用智能诊断结果
        renderQuickDiagnosis(quickDiagnosis);
        
        // 使用智能诊断的泄漏嫌疑
        const leakSuspects = diagnosisResult.leakSuspects.length > 0 
            ? diagnosisResult.leakSuspects 
            : quickDiagnosis.leak_suspects || [];
        renderLeakSuspects(leakSuspects, topClasses, suggestions);
        
        // 使用智能诊断的业务类
        const businessClasses = diagnosisResult.businessClasses.length > 0
            ? diagnosisResult.businessClasses
            : quickDiagnosis.top_business_classes || [];
        renderBusinessClasses(businessClasses);
        
        // 使用智能诊断的集合问题
        const collectionIssues = diagnosisResult.collectionIssues.length > 0
            ? diagnosisResult.collectionIssues
            : quickDiagnosis.collection_issues || [];
        renderCollectionIssues(collectionIssues);
        
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
            // 优先从 App 获取当前任务 ID，回退到 URL 参数
            const taskId = (typeof App !== 'undefined' && App.getCurrentTask()) 
                || new URLSearchParams(window.location.search).get('task') 
                || '';
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
        if (typeof showPanel === 'function') {
            showPanel('heaphistogram');
        } else if (typeof App !== 'undefined') {
            App.showPanel('heaphistogram');
        }
        if (typeof HeapHistogram !== 'undefined') {
            HeapHistogram.searchClass(className);
        }
    }

    /**
     * 查看持有者（跳转到 Merged Paths）
     * @param {string} className - 类名
     */
    function viewRetainers(className) {
        if (typeof showPanel === 'function') {
            showPanel('heapmergedpaths');
        } else if (typeof App !== 'undefined') {
            App.showPanel('heapmergedpaths');
        }
        HeapCore.showNotification(`查看 ${getShortClassName(className)} 的持有者`, 'info');
    }

    /**
     * 在引用图中查看（已废弃，跳转到 Merged Paths）
     * @param {string} className - 类名
     */
    function viewInRefGraph(className) {
        viewRetainers(className);
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
        viewRetainers,
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
