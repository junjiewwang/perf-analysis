/**
 * Heap Analysis Core Module
 * 核心模块：状态管理、工具函数、模块注册、初始化
 * 
 * 设计原则：
 * - 单一职责：只负责状态管理和模块协调
 * - 开放封闭：通过事件系统扩展，不修改核心代码
 * - 依赖倒置：子模块依赖抽象接口，不依赖具体实现
 */

const HeapCore = (function() {
    'use strict';

    // ============================================
    // 私有状态 (Private State)
    // ============================================
    
    const state = {
        // 原始数据
        classData: [],
        topItems: [],
        heapData: {},
        
        // 引用关系数据
        referenceGraphs: {},
        businessRetainers: {},
        gcRootPaths: {},
        
        // 统计信息
        totalHeapSize: 0,
        maxShallowSize: 0,
        maxRetainedSize: 0,
        
        // UI 状态
        viewMode: 'flat',
        histogramSortField: 'shallow',
        histogramSortAsc: false
    };

    // 已注册的子模块
    const modules = new Map();
    
    // 事件监听器
    const eventListeners = new Map();

    // ============================================
    // 事件系统 (Event System)
    // ============================================
    
    /**
     * 订阅事件
     * @param {string} event - 事件名称
     * @param {Function} callback - 回调函数
     */
    function on(event, callback) {
        if (!eventListeners.has(event)) {
            eventListeners.set(event, []);
        }
        eventListeners.get(event).push(callback);
    }

    /**
     * 取消订阅
     * @param {string} event - 事件名称
     * @param {Function} callback - 回调函数
     */
    function off(event, callback) {
        if (!eventListeners.has(event)) return;
        const listeners = eventListeners.get(event);
        const idx = listeners.indexOf(callback);
        if (idx > -1) listeners.splice(idx, 1);
    }

    /**
     * 触发事件
     * @param {string} event - 事件名称
     * @param {*} data - 事件数据
     */
    function emit(event, data) {
        if (!eventListeners.has(event)) return;
        eventListeners.get(event).forEach(cb => {
            try {
                cb(data);
            } catch (e) {
                console.error(`[HeapCore] Event handler error for ${event}:`, e);
            }
        });
    }

    // ============================================
    // 状态管理 (State Management)
    // ============================================
    
    /**
     * 获取状态（只读副本）
     * @param {string} key - 状态键名，可选
     * @returns {*} 状态值
     */
    function getState(key) {
        if (key) {
            return state[key];
        }
        return { ...state };
    }

    /**
     * 更新状态
     * @param {string} key - 状态键名
     * @param {*} value - 新值
     */
    function setState(key, value) {
        const oldValue = state[key];
        state[key] = value;
        emit('stateChange', { key, oldValue, newValue: value });
    }

    /**
     * 批量更新状态
     * @param {Object} updates - 要更新的键值对
     */
    function setStateMultiple(updates) {
        const changes = [];
        for (const [key, value] of Object.entries(updates)) {
            const oldValue = state[key];
            state[key] = value;
            changes.push({ key, oldValue, newValue: value });
        }
        emit('stateChange', { batch: true, changes });
    }

    // ============================================
    // 模块注册 (Module Registration)
    // ============================================
    
    /**
     * 注册子模块
     * @param {string} name - 模块名称
     * @param {Object} module - 模块对象
     */
    function registerModule(name, module) {
        if (modules.has(name)) {
            console.warn(`[HeapCore] Module "${name}" already registered, overwriting.`);
        }
        modules.set(name, module);
        
        // 如果模块有初始化方法，调用它
        if (typeof module.init === 'function') {
            try {
                module.init();
            } catch (e) {
                console.error(`[HeapCore] Failed to initialize module "${name}":`, e);
            }
        }
        
        emit('moduleRegistered', { name, module });
    }

    /**
     * 获取已注册的模块
     * @param {string} name - 模块名称
     * @returns {Object|null} 模块对象
     */
    function getModule(name) {
        return modules.get(name) || null;
    }

    // ============================================
    // 数据处理 (Data Processing)
    // ============================================
    
    /**
     * 加载并处理分析数据
     * @param {Object} data - 原始分析数据
     */
    function loadAnalysisData(data) {
        console.log('[HeapCore] loadAnalysisData called with data:', data ? 'present' : 'null');
        if (!data) {
            console.error('[HeapCore] loadAnalysisData: data is null or undefined');
            return;
        }
        console.log('[HeapCore] loadAnalysisData: data keys:', Object.keys(data));
        
        const topItems = data.top_items || [];
        const heapData = data.data || {};
        const topClasses = heapData.top_classes || [];
        
        console.log('[HeapCore] topItems:', topItems.length, 'topClasses:', topClasses.length);
        if (topItems.length > 0) {
            console.log('[HeapCore] First topItem:', topItems[0]);
        }
        if (topClasses.length > 0) {
            console.log('[HeapCore] First topClass:', topClasses[0]);
        }

        // 构建 retainer 映射和 GC root paths 映射
        const retainerMap = {};
        const gcRootPathsMap = {};
        
        topClasses.forEach(cls => {
            const className = cls.class_name || cls.name;
            if (!className) return;
            
            if (cls.retainers && cls.retainers.length > 0) {
                retainerMap[className] = cls.retainers;
            }
            if (cls.gc_root_paths && cls.gc_root_paths.length > 0) {
                gcRootPathsMap[className] = cls.gc_root_paths;
            }
        });

        // 合并所有类数据
        const allClassData = [];
        const seenClasses = new Set();
        
        // 首先添加 topItems（包含完整信息）
        topItems.forEach(item => {
            const classInfo = topClasses.find(c => (c.class_name || c.name) === item.name) || {};
            seenClasses.add(item.name);
            
            const gcPaths = classInfo.gc_root_paths || gcRootPathsMap[item.name] || [];
            
            allClassData.push({
                name: item.name,
                size: item.value,
                percentage: item.percentage,
                instanceCount: item.extra ? item.extra.instance_count : (classInfo.instance_count || 0),
                count: item.extra ? item.extra.instance_count : (classInfo.instance_count || 0),
                retainers: classInfo.retainers || retainerMap[item.name] || [],
                retained_size: classInfo.retained_size || 0,
                gc_root_paths: gcPaths
            });
            
            // 确保 gcRootPathsMap 中有这个类
            if (gcPaths.length > 0) {
                gcRootPathsMap[item.name] = gcPaths;
            }
        });
        
        // 添加未包含在 topItems 中的类
        topClasses.forEach(cls => {
            const className = cls.class_name || cls.name;
            if (!className || seenClasses.has(className)) return;
            
            const gcPaths = cls.gc_root_paths || [];
            
            allClassData.push({
                name: className,
                size: cls.total_size || cls.size || 0,
                percentage: cls.percentage || 0,
                instanceCount: cls.instance_count || 0,
                count: cls.instance_count || 0,
                retainers: cls.retainers || [],
                retained_size: cls.retained_size || 0,
                gc_root_paths: gcPaths
            });
            
            // 确保 gcRootPathsMap 中有这个类
            if (gcPaths.length > 0) {
                gcRootPathsMap[className] = gcPaths;
            }
        });

        // 计算统计值
        const maxShallow = Math.max(...allClassData.map(c => c.size), 0);
        const maxRetained = Math.max(...allClassData.map(c => c.retained_size || 0), 0);

        console.log('[HeapCore] Loaded data:', {
            classCount: allClassData.length,
            classesWithPaths: Object.keys(gcRootPathsMap).length,
            firstClass: allClassData.length > 0 ? allClassData[0] : null
        });

        // 批量更新状态
        setStateMultiple({
            classData: allClassData,
            topItems: topItems,
            heapData: heapData,
            referenceGraphs: heapData.reference_graphs || {},
            businessRetainers: heapData.business_retainers || {},
            gcRootPaths: gcRootPathsMap,
            totalHeapSize: heapData.total_heap_size || 0,
            maxShallowSize: maxShallow,
            maxRetainedSize: maxRetained
        });

        // 触发数据加载完成事件
        emit('dataLoaded', { classData: allClassData, heapData, topItems, gcRootPaths: gcRootPathsMap });
    }

    /**
     * 更新详细的 retainer 数据
     * @param {Object} data - retainer 数据
     */
    function updateRetainerData(data) {
        if (data.business_retainers) {
            setState('businessRetainers', data.business_retainers);
        }
        if (data.reference_graphs) {
            setState('referenceGraphs', data.reference_graphs);
        }
        if (data.top_classes) {
            const classData = getState('classData');
            data.top_classes.forEach(cls => {
                const existing = classData.find(c => c.name === cls.class_name);
                if (existing) {
                    existing.retainers = cls.retainers || [];
                    existing.gc_root_paths = cls.gc_root_paths || [];
                }
            });
            setState('classData', classData);
        }
        
        emit('retainerDataUpdated', data);
    }

    // ============================================
    // 工具函数 (Utility Functions)
    // ============================================
    
    /**
     * 格式化类名（IDEA 风格）
     * @param {string} fullName - 完整类名
     * @returns {string} HTML 格式化的类名
     */
    function formatClassNameIDEA(fullName) {
        const lastDot = fullName.lastIndexOf('.');
        if (lastDot === -1) {
            return `<span class="simple-name">${Utils.escapeHtml(fullName)}</span>`;
        }
        const packagePart = fullName.substring(0, lastDot + 1);
        const simpleName = fullName.substring(lastDot + 1);
        return `<span class="package">${Utils.escapeHtml(packagePart)}</span><span class="simple-name">${Utils.escapeHtml(simpleName)}</span>`;
    }

    /**
     * 检查是否为业务类
     * @param {string} className - 类名
     * @returns {boolean}
     */
    function isBusinessClass(className) {
        if (!className) return false;
        return !className.startsWith('java.') && 
               !className.startsWith('javax.') &&
               !className.startsWith('sun.') && 
               !className.startsWith('jdk.') &&
               !className.startsWith('com.sun.') &&
               !className.includes('[]');
    }

    /**
     * 按包名分组类数据
     * @param {Array} classData - 类数据数组
     * @returns {Map} 包名到类列表的映射
     */
    function groupByPackage(classData) {
        const packageMap = new Map();
        
        classData.forEach(cls => {
            const parts = cls.name.split('.');
            let packageName = 'default';
            
            if (parts.length > 1) {
                parts.pop();
                packageName = parts.join('.');
            }

            if (!packageMap.has(packageName)) {
                packageMap.set(packageName, { 
                    totalSize: 0, 
                    totalInstances: 0, 
                    classes: [] 
                });
            }

            const pkg = packageMap.get(packageName);
            pkg.totalSize += cls.size;
            pkg.totalInstances += cls.instanceCount;
            pkg.classes.push(cls);
        });

        return packageMap;
    }

    /**
     * 显示通知消息
     * @param {string} message - 消息内容
     * @param {string} type - 消息类型 ('success' | 'warning' | 'error')
     */
    function showNotification(message, type = 'success') {
        // 移除已存在的通知
        const existing = document.querySelector('.heap-notification');
        if (existing) existing.remove();
        
        const notification = document.createElement('div');
        notification.className = `heap-notification ${type}`;
        notification.innerHTML = `<span>${message}</span>`;
        notification.style.cssText = `
            position: fixed;
            top: 80px;
            right: 20px;
            padding: 10px 20px;
            border-radius: 6px;
            background: ${type === 'success' ? '#4caf50' : type === 'error' ? '#f44336' : '#ff9800'};
            color: white;
            font-size: 14px;
            z-index: 1000;
            box-shadow: 0 2px 10px rgba(0,0,0,0.2);
            animation: slideIn 0.3s ease;
        `;
        document.body.appendChild(notification);
        
        setTimeout(() => notification.remove(), 3000);
    }

    // ============================================
    // 初始化 (Initialization)
    // ============================================
    
    /**
     * 初始化核心模块
     */
    function init() {
        // 初始化搜索事件监听
        const searchInput = document.getElementById('heapClassSearch');
        if (searchInput) {
            searchInput.addEventListener('keyup', () => {
                emit('searchChanged', searchInput.value);
            });
        }
        
        emit('coreInitialized');
    }

    // ============================================
    // 公共 API (Public API)
    // ============================================
    
    return {
        // 初始化
        init,
        
        // 状态管理
        getState,
        setState,
        setStateMultiple,
        
        // 模块管理
        registerModule,
        getModule,
        
        // 事件系统
        on,
        off,
        emit,
        
        // 数据处理
        loadAnalysisData,
        updateRetainerData,
        
        // 工具函数
        formatClassNameIDEA,
        isBusinessClass,
        groupByPackage,
        showNotification
    };
})();

// 导出到全局
window.HeapCore = HeapCore;
