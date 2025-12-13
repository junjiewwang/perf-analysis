/**
 * Heap Diagnosis Module
 * 问题诊断概览模块：首页直接展示问题结论
 * 
 * 设计理念：
 * - 用户打开页面第一眼就能看到问题
 * - 像"资深 SRE 同事"一样直接告诉用户问题在哪
 * - 提供可执行的具体建议，而非通用建议
 */

const HeapDiagnosis = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let diagnosisData = null;
    let topClasses = [];
    let suggestions = [];

    // ============================================
    // 诊断规则引擎
    // ============================================

    /**
     * 执行完整诊断分析
     * @param {Object} data - 原始分析数据
     * @returns {Object} 诊断结果
     */
    function runDiagnosis(data) {
        topClasses = data.data?.top_classes || [];
        suggestions = data.suggestions || [];
        const heapData = data.data || {};
        const totalHeapSize = heapData.total_heap_size || 0;

        const diagnosis = {
            severity: 'normal',  // critical, warning, normal
            issues: [],
            summary: null,
            recommendations: []
        };

        // 规则 1: 检测大内存消费者
        const bigConsumers = detectBigConsumers(topClasses, totalHeapSize);
        diagnosis.issues.push(...bigConsumers);

        // 规则 2: 检测潜在内存泄漏
        const leakSuspects = detectLeakSuspects(topClasses, heapData);
        diagnosis.issues.push(...leakSuspects);

        // 规则 3: 检测集合类问题
        const collectionIssues = detectCollectionIssues(topClasses);
        diagnosis.issues.push(...collectionIssues);

        // 规则 4: 检测字符串/byte[]问题
        const primitiveIssues = detectPrimitiveIssues(topClasses, totalHeapSize);
        diagnosis.issues.push(...primitiveIssues);

        // 规则 5: 检测业务类问题
        const businessIssues = detectBusinessClassIssues(topClasses, totalHeapSize);
        diagnosis.issues.push(...businessIssues);

        // 按严重程度排序
        diagnosis.issues.sort((a, b) => {
            const severityOrder = { critical: 0, warning: 1, info: 2 };
            return severityOrder[a.severity] - severityOrder[b.severity];
        });

        // 确定整体严重程度
        if (diagnosis.issues.some(i => i.severity === 'critical')) {
            diagnosis.severity = 'critical';
        } else if (diagnosis.issues.some(i => i.severity === 'warning')) {
            diagnosis.severity = 'warning';
        }

        // 生成摘要
        diagnosis.summary = generateSummary(diagnosis, totalHeapSize);

        // 生成具体建议
        diagnosis.recommendations = generateRecommendations(diagnosis.issues);

        return diagnosis;
    }

    /**
     * 检测大内存消费者
     */
    function detectBigConsumers(classes, totalHeapSize) {
        const issues = [];
        
        for (const cls of classes.slice(0, 10)) {
            const percentage = cls.percentage || 0;
            const className = cls.class_name || cls.name || '';
            
            if (percentage > 30) {
                issues.push({
                    severity: 'critical',
                    type: 'big_consumer',
                    title: `${getShortClassName(className)} 占用 ${percentage.toFixed(1)}% 堆内存`,
                    description: `单个类占用超过 30% 的堆内存，这是异常情况`,
                    className: className,
                    metrics: {
                        size: cls.total_size || cls.size,
                        percentage: percentage,
                        instanceCount: cls.instance_count
                    },
                    retainers: cls.retainers || [],
                    rootCause: analyzeRootCause(cls),
                    actions: generateActionsForClass(cls)
                });
            } else if (percentage > 15) {
                issues.push({
                    severity: 'warning',
                    type: 'big_consumer',
                    title: `${getShortClassName(className)} 占用 ${percentage.toFixed(1)}% 堆内存`,
                    description: `内存占用较高，建议检查是否合理`,
                    className: className,
                    metrics: {
                        size: cls.total_size || cls.size,
                        percentage: percentage,
                        instanceCount: cls.instance_count
                    },
                    retainers: cls.retainers || [],
                    rootCause: analyzeRootCause(cls),
                    actions: generateActionsForClass(cls)
                });
            }
        }
        
        return issues;
    }

    /**
     * 检测潜在内存泄漏
     */
    function detectLeakSuspects(classes, heapData) {
        const issues = [];
        
        for (const cls of classes.slice(0, 20)) {
            const className = cls.class_name || cls.name || '';
            const instanceCount = cls.instance_count || 0;
            const retainers = cls.retainers || [];
            
            // 检查是否有 static 字段持有
            const hasStaticRetainer = retainers.some(r => 
                r.field_name?.includes('static') || 
                r.retainer_class?.includes('$') === false && r.depth === 1
            );
            
            // 检查是否是缓存类
            const isCacheClass = className.toLowerCase().includes('cache') ||
                                 className.toLowerCase().includes('pool') ||
                                 className.toLowerCase().includes('registry');
            
            // 检查实例数是否异常
            const hasHighInstanceCount = instanceCount > 50000;
            
            if (hasStaticRetainer && (cls.percentage > 10 || isCacheClass)) {
                issues.push({
                    severity: 'critical',
                    type: 'leak_suspect',
                    title: `疑似内存泄漏: ${getShortClassName(className)}`,
                    description: `被 static 字段持有，且占用大量内存，可能无法被 GC 回收`,
                    className: className,
                    metrics: {
                        size: cls.total_size || cls.size,
                        percentage: cls.percentage,
                        instanceCount: instanceCount
                    },
                    retainers: retainers,
                    rootCause: {
                        type: 'static_reference',
                        detail: '对象被 static 字段持有，生命周期与应用相同'
                    },
                    actions: [
                        {
                            type: 'check_lifecycle',
                            label: '检查对象生命周期',
                            detail: '确认是否需要长期持有这些对象'
                        },
                        {
                            type: 'add_cleanup',
                            label: '添加清理机制',
                            detail: '考虑使用 WeakReference 或添加过期清理'
                        }
                    ]
                });
            } else if (hasHighInstanceCount && cls.percentage > 5) {
                issues.push({
                    severity: 'warning',
                    type: 'high_instance_count',
                    title: `实例数异常: ${getShortClassName(className)}`,
                    description: `${Utils.formatNumber(instanceCount)} 个实例，可能存在对象创建过多问题`,
                    className: className,
                    metrics: {
                        size: cls.total_size || cls.size,
                        percentage: cls.percentage,
                        instanceCount: instanceCount
                    },
                    retainers: retainers,
                    rootCause: analyzeRootCause(cls),
                    actions: [
                        {
                            type: 'check_creation',
                            label: '检查对象创建',
                            detail: '确认是否有不必要的对象创建'
                        },
                        {
                            type: 'use_pool',
                            label: '考虑对象池',
                            detail: '对于频繁创建的对象，使用对象池复用'
                        }
                    ]
                });
            }
        }
        
        return issues;
    }

    /**
     * 检测集合类问题
     */
    function detectCollectionIssues(classes) {
        const issues = [];
        const collectionClasses = ['HashMap', 'ArrayList', 'LinkedList', 'HashSet', 
                                   'ConcurrentHashMap', 'LinkedHashMap', 'TreeMap'];
        
        for (const cls of classes) {
            const className = cls.class_name || cls.name || '';
            const instanceCount = cls.instance_count || 0;
            
            const isCollection = collectionClasses.some(c => className.includes(c));
            
            if (isCollection && instanceCount > 10000) {
                const severity = instanceCount > 100000 ? 'critical' : 'warning';
                
                issues.push({
                    severity: severity,
                    type: 'collection_issue',
                    title: `集合类实例过多: ${getShortClassName(className)}`,
                    description: `${Utils.formatNumber(instanceCount)} 个实例，可能存在集合未清理或重复创建问题`,
                    className: className,
                    metrics: {
                        size: cls.total_size || cls.size,
                        percentage: cls.percentage,
                        instanceCount: instanceCount
                    },
                    retainers: cls.retainers || [],
                    rootCause: {
                        type: 'collection_accumulation',
                        detail: '集合对象累积，可能是缓存未清理或循环中创建集合'
                    },
                    actions: [
                        {
                            type: 'check_lifecycle',
                            label: '检查集合生命周期',
                            detail: '确认集合是否在使用后被正确清理'
                        },
                        {
                            type: 'check_creation_point',
                            label: '检查创建位置',
                            detail: '确认是否在循环中创建集合对象'
                        }
                    ]
                });
            }
        }
        
        return issues;
    }

    /**
     * 检测基本类型问题 (String, byte[])
     * 重点：向上追溯到业务层，而非停留在底层类型
     */
    function detectPrimitiveIssues(classes, totalHeapSize) {
        const issues = [];
        
        for (const cls of classes) {
            const className = cls.class_name || cls.name || '';
            const size = cls.total_size || cls.size || 0;
            const percentage = cls.percentage || 0;
            const instanceCount = cls.instance_count || 0;
            const retainers = cls.retainers || [];
            
            if (className === 'byte[]' && percentage > 20) {
                // 分析 byte[] 的真正来源
                const sourceAnalysis = analyzeByteArraySource(retainers, classes);
                
                issues.push({
                    severity: percentage > 40 ? 'critical' : 'warning',
                    type: 'byte_array_issue',
                    title: sourceAnalysis.title,
                    description: sourceAnalysis.description,
                    className: className,
                    metrics: { size, percentage, instanceCount },
                    retainers: retainers,
                    rootCause: sourceAnalysis.rootCause,
                    businessContext: sourceAnalysis.businessContext,
                    actions: sourceAnalysis.actions
                });
            }
            
            if ((className === 'java.lang.String' || className === 'String') && instanceCount > 500000) {
                // 分析 String 的真正来源
                const sourceAnalysis = analyzeStringSource(retainers, classes);
                
                issues.push({
                    severity: 'warning',
                    type: 'string_issue',
                    title: sourceAnalysis.title,
                    description: sourceAnalysis.description,
                    className: className,
                    metrics: { size, percentage, instanceCount },
                    retainers: retainers,
                    rootCause: sourceAnalysis.rootCause,
                    businessContext: sourceAnalysis.businessContext,
                    actions: sourceAnalysis.actions
                });
            }
        }
        
        return issues;
    }

    /**
     * 分析 byte[] 的真正来源
     * 向上追溯到业务层，识别具体场景
     */
    function analyzeByteArraySource(retainers, allClasses) {
        const result = {
            title: 'byte[] 内存占用过高',
            description: '需要进一步分析来源',
            rootCause: { type: 'unknown', detail: '无法确定来源' },
            businessContext: null,
            actions: []
        };

        if (!retainers || retainers.length === 0) {
            result.description = '无法获取持有者信息，建议查看 Merged Paths 进行深入分析';
            result.actions = [{ type: 'view_retainers', label: '查看引用路径', detail: '分析 byte[] 的持有链' }];
            return result;
        }

        // 分析持有者模式
        const retainerPatterns = analyzeRetainerPatterns(retainers);
        
        // 场景 1: Netty 内存池 (PoolChunk, PoolArena)
        if (retainerPatterns.hasNettyPool) {
            result.title = 'Netty 内存池占用大量内存';
            result.description = '这是 Netty 的正常内存池机制，byte[] 被 PoolChunk 管理用于网络 I/O';
            result.rootCause = {
                type: 'netty_pool',
                detail: 'Netty 使用内存池优化网络 I/O 性能，这通常是正常的'
            };
            result.businessContext = {
                framework: 'Netty',
                usage: '网络通信缓冲区',
                suggestion: '检查是否有连接泄漏或请求积压'
            };
            result.actions = [
                { type: 'check_connections', label: '检查连接数', detail: '确认是否有连接泄漏' },
                { type: 'check_request_queue', label: '检查请求队列', detail: '是否有请求积压导致缓冲区累积' },
                { type: 'tune_pool', label: '调整内存池', detail: '可通过 -Dio.netty.allocator.* 调整' }
            ];
            
            // 尝试找到使用 Netty 的业务类
            const businessUser = findBusinessUserOfFramework(allClasses, ['netty', 'channel', 'handler']);
            if (businessUser) {
                result.businessContext.businessClass = businessUser;
                result.description += `。业务入口可能是: ${getShortClassName(businessUser)}`;
            }
            return result;
        }

        // 场景 2: 图片/媒体处理
        if (retainerPatterns.hasImageProcessing) {
            result.title = '图片/媒体数据占用大量内存';
            result.description = 'byte[] 被图片处理相关类持有，可能是图片缓存或处理中的数据';
            result.rootCause = {
                type: 'image_processing',
                detail: `被 ${getShortClassName(retainerPatterns.imageClass)} 持有`
            };
            result.actions = [
                { type: 'check_image_cache', label: '检查图片缓存', detail: '确认缓存策略是否合理' },
                { type: 'check_image_size', label: '检查图片大小', detail: '是否有超大图片未压缩' }
            ];
            return result;
        }

        // 场景 3: 序列化/反序列化
        if (retainerPatterns.hasSerialization) {
            result.title = '序列化数据占用大量内存';
            result.description = 'byte[] 来自序列化操作，可能是消息队列、RPC 调用或缓存序列化';
            result.rootCause = {
                type: 'serialization',
                detail: `被 ${getShortClassName(retainerPatterns.serializationClass)} 持有`
            };
            result.actions = [
                { type: 'check_message_size', label: '检查消息大小', detail: '是否有超大消息' },
                { type: 'check_batch_size', label: '检查批量大小', detail: '批量处理是否过大' }
            ];
            return result;
        }

        // 场景 4: 文件/IO 操作
        if (retainerPatterns.hasFileIO) {
            result.title = '文件/IO 缓冲区占用大量内存';
            result.description = 'byte[] 来自文件读写操作，可能是大文件处理或流未关闭';
            result.rootCause = {
                type: 'file_io',
                detail: `被 ${getShortClassName(retainerPatterns.ioClass)} 持有`
            };
            result.actions = [
                { type: 'check_stream_close', label: '检查流关闭', detail: '确认 InputStream/OutputStream 是否正确关闭' },
                { type: 'check_file_size', label: '检查文件大小', detail: '是否一次性读取大文件' }
            ];
            return result;
        }

        // 场景 5: 缓存
        if (retainerPatterns.hasCache) {
            result.title = '缓存数据占用大量内存';
            result.description = `byte[] 被缓存持有: ${getShortClassName(retainerPatterns.cacheClass)}`;
            result.rootCause = {
                type: 'cache',
                detail: `缓存 ${getShortClassName(retainerPatterns.cacheClass)} 持有大量数据`
            };
            result.actions = [
                { type: 'check_cache_size', label: '检查缓存大小', detail: '确认缓存是否设置了大小限制' },
                { type: 'check_cache_ttl', label: '检查过期策略', detail: '确认缓存是否有过期清理机制' }
            ];
            return result;
        }

        // 默认：显示直接持有者，但提示需要进一步分析
        const topRetainer = retainers[0];
        result.title = `byte[] 被 ${getShortClassName(topRetainer.retainer_class)} 持有`;
        result.description = '这是一个底层持有者，需要继续向上追溯找到业务代码';
        result.rootCause = {
            type: 'needs_investigation',
            detail: `直接持有者: ${getShortClassName(topRetainer.retainer_class)}.${topRetainer.field_name || '?'}`
        };
        result.actions = [
            { type: 'view_retainers', label: '查看完整引用链', detail: '在 Merged Paths 中追溯到业务代码' },
            { type: 'search', label: '搜索持有者类', detail: '查看持有者的详细信息' }
        ];

        return result;
    }

    /**
     * 分析 String 的真正来源
     */
    function analyzeStringSource(retainers, allClasses) {
        const result = {
            title: 'String 对象过多',
            description: '需要进一步分析来源',
            rootCause: { type: 'unknown', detail: '无法确定来源' },
            businessContext: null,
            actions: []
        };

        if (!retainers || retainers.length === 0) {
            result.description = '无法获取持有者信息，可能是日志、配置或业务数据';
            result.actions = [
                { type: 'view_retainers', label: '查看引用路径', detail: '分析 String 的持有链' },
                { type: 'use_stringbuilder', label: '使用 StringBuilder', detail: '优化字符串拼接' }
            ];
            return result;
        }

        const retainerPatterns = analyzeRetainerPatterns(retainers);

        // 场景 1: 日志相关
        if (retainerPatterns.hasLogging) {
            result.title = '日志字符串占用大量内存';
            result.description = 'String 被日志框架持有，可能是日志缓冲区过大或异步日志积压';
            result.rootCause = {
                type: 'logging',
                detail: `被 ${getShortClassName(retainerPatterns.loggingClass)} 持有`
            };
            result.actions = [
                { type: 'check_log_level', label: '检查日志级别', detail: '生产环境避免 DEBUG 级别' },
                { type: 'check_async_log', label: '检查异步日志', detail: '确认异步日志队列大小' }
            ];
            return result;
        }

        // 场景 2: 缓存
        if (retainerPatterns.hasCache) {
            result.title = '缓存字符串占用大量内存';
            result.description = `String 被缓存持有: ${getShortClassName(retainerPatterns.cacheClass)}`;
            result.rootCause = {
                type: 'cache',
                detail: `缓存 ${getShortClassName(retainerPatterns.cacheClass)} 持有大量字符串`
            };
            result.actions = [
                { type: 'check_cache_size', label: '检查缓存大小', detail: '确认缓存是否设置了大小限制' },
                { type: 'intern_strings', label: '考虑 String.intern()', detail: '对于重复字符串使用 intern()' }
            ];
            return result;
        }

        // 默认
        result.description = '大量 String 对象，可能来自业务数据处理或字符串拼接';
        result.actions = [
            { type: 'view_retainers', label: '查看引用路径', detail: '分析 String 的持有链' },
            { type: 'use_stringbuilder', label: '使用 StringBuilder', detail: '优化字符串拼接' }
        ];

        return result;
    }

    /**
     * 分析持有者模式，识别常见框架和场景
     */
    function analyzeRetainerPatterns(retainers) {
        const patterns = {
            hasNettyPool: false,
            hasImageProcessing: false,
            hasSerialization: false,
            hasFileIO: false,
            hasCache: false,
            hasLogging: false,
            nettyClass: null,
            imageClass: null,
            serializationClass: null,
            ioClass: null,
            cacheClass: null,
            loggingClass: null
        };

        for (const retainer of retainers) {
            const cls = (retainer.retainer_class || '').toLowerCase();
            const field = (retainer.field_name || '').toLowerCase();

            // Netty 内存池
            if (cls.includes('poolchunk') || cls.includes('poolarena') || 
                cls.includes('pooled') || cls.includes('io.netty')) {
                patterns.hasNettyPool = true;
                patterns.nettyClass = retainer.retainer_class;
            }

            // 图片处理
            if (cls.includes('image') || cls.includes('bitmap') || 
                cls.includes('picture') || cls.includes('thumbnail')) {
                patterns.hasImageProcessing = true;
                patterns.imageClass = retainer.retainer_class;
            }

            // 序列化
            if (cls.includes('serial') || cls.includes('protobuf') || 
                cls.includes('kryo') || cls.includes('hessian') ||
                cls.includes('jackson') || cls.includes('gson')) {
                patterns.hasSerialization = true;
                patterns.serializationClass = retainer.retainer_class;
            }

            // 文件 IO
            if (cls.includes('stream') || cls.includes('buffer') ||
                cls.includes('file') || cls.includes('channel')) {
                patterns.hasFileIO = true;
                patterns.ioClass = retainer.retainer_class;
            }

            // 缓存
            if (cls.includes('cache') || field.includes('cache') ||
                cls.includes('caffeine') || cls.includes('guava') ||
                cls.includes('ehcache') || cls.includes('redis')) {
                patterns.hasCache = true;
                patterns.cacheClass = retainer.retainer_class;
            }

            // 日志
            if (cls.includes('log') || cls.includes('appender') ||
                cls.includes('slf4j') || cls.includes('logback') ||
                cls.includes('log4j')) {
                patterns.hasLogging = true;
                patterns.loggingClass = retainer.retainer_class;
            }
        }

        return patterns;
    }

    /**
     * 尝试找到使用某个框架的业务类
     */
    function findBusinessUserOfFramework(allClasses, frameworkKeywords) {
        for (const cls of allClasses) {
            const className = cls.class_name || cls.name || '';
            
            // 跳过 JDK 和框架类
            if (isJDKClass(className) || isFrameworkClass(className)) {
                continue;
            }

            // 检查 retainers 中是否有框架类
            const retainers = cls.retainers || [];
            for (const retainer of retainers) {
                const retainerClass = (retainer.retainer_class || '').toLowerCase();
                if (frameworkKeywords.some(kw => retainerClass.includes(kw))) {
                    return className;
                }
            }
        }
        return null;
    }

    /**
     * 检测业务类问题
     */
    function detectBusinessClassIssues(classes, totalHeapSize) {
        const issues = [];
        
        for (const cls of classes.slice(0, 30)) {
            const className = cls.class_name || cls.name || '';
            const percentage = cls.percentage || 0;
            
            // 跳过 JDK 和框架类
            if (isJDKClass(className) || isFrameworkClass(className)) {
                continue;
            }
            
            if (percentage > 5) {
                issues.push({
                    severity: percentage > 15 ? 'warning' : 'info',
                    type: 'business_class',
                    title: `业务类内存占用: ${getShortClassName(className)}`,
                    description: `业务类占用 ${percentage.toFixed(1)}% 堆内存，需要关注`,
                    className: className,
                    metrics: {
                        size: cls.total_size || cls.size,
                        percentage: percentage,
                        instanceCount: cls.instance_count
                    },
                    retainers: cls.retainers || [],
                    rootCause: analyzeRootCause(cls),
                    actions: generateActionsForClass(cls)
                });
            }
        }
        
        return issues;
    }

    /**
     * 分析根因
     */
    function analyzeRootCause(cls) {
        const retainers = cls.retainers || [];
        
        if (retainers.length === 0) {
            return {
                type: 'unknown',
                detail: '无法确定持有者，需要进一步分析'
            };
        }
        
        const topRetainer = retainers[0];
        const retainerClass = topRetainer.retainer_class || '';
        const fieldName = topRetainer.field_name || '';
        
        // 检查是否是缓存
        if (retainerClass.toLowerCase().includes('cache') || 
            fieldName.toLowerCase().includes('cache')) {
            return {
                type: 'cache',
                detail: `被缓存持有: ${getShortClassName(retainerClass)}.${fieldName}`,
                retainer: topRetainer
            };
        }
        
        // 检查是否是集合
        if (retainerClass.includes('Map') || retainerClass.includes('List') || 
            retainerClass.includes('Set')) {
            return {
                type: 'collection',
                detail: `被集合持有: ${getShortClassName(retainerClass)}`,
                retainer: topRetainer
            };
        }
        
        // 检查是否是静态字段
        if (topRetainer.depth === 1) {
            return {
                type: 'static_reference',
                detail: `被静态字段持有: ${getShortClassName(retainerClass)}.${fieldName}`,
                retainer: topRetainer
            };
        }
        
        return {
            type: 'reference_chain',
            detail: `引用链深度 ${topRetainer.depth}: ${getShortClassName(retainerClass)}`,
            retainer: topRetainer
        };
    }

    /**
     * 为类生成操作建议
     */
    function generateActionsForClass(cls) {
        const className = cls.class_name || cls.name || '';
        const actions = [];
        
        actions.push({
            type: 'search',
            label: '在 Histogram 中搜索',
            detail: '查看详细的类信息和 Retainer'
        });
        
        if (cls.retainers && cls.retainers.length > 0) {
            actions.push({
                type: 'view_retainers',
                label: '查看持有者',
                detail: '分析谁持有了这些对象'
            });
        }
        
        return actions;
    }

    /**
     * 生成诊断摘要
     */
    function generateSummary(diagnosis, totalHeapSize) {
        const criticalCount = diagnosis.issues.filter(i => i.severity === 'critical').length;
        const warningCount = diagnosis.issues.filter(i => i.severity === 'warning').length;
        
        if (criticalCount > 0) {
            return {
                icon: '🔴',
                text: `检测到 ${criticalCount} 个严重问题${warningCount > 0 ? `，${warningCount} 个警告` : ''}`,
                subtext: `堆大小: ${Utils.formatBytes(totalHeapSize)}，建议立即处理`
            };
        } else if (warningCount > 0) {
            return {
                icon: '🟡',
                text: `检测到 ${warningCount} 个潜在问题`,
                subtext: `堆大小: ${Utils.formatBytes(totalHeapSize)}，建议关注`
            };
        } else {
            return {
                icon: '🟢',
                text: '未检测到明显问题',
                subtext: `堆大小: ${Utils.formatBytes(totalHeapSize)}，内存使用正常`
            };
        }
    }

    /**
     * 生成具体建议
     */
    function generateRecommendations(issues) {
        const recommendations = [];
        const seenTypes = new Set();
        
        for (const issue of issues) {
            if (seenTypes.has(issue.type)) continue;
            seenTypes.add(issue.type);
            
            switch (issue.type) {
                case 'leak_suspect':
                    recommendations.push({
                        priority: 1,
                        title: '检查内存泄漏',
                        detail: '发现对象被 static 字段持有，建议检查对象生命周期，添加清理机制或使用 WeakReference'
                    });
                    break;
                case 'collection_issue':
                    recommendations.push({
                        priority: 2,
                        title: '优化集合使用',
                        detail: '集合类实例过多，检查是否在循环中创建集合，确保集合在使用后被清理'
                    });
                    break;
                case 'byte_array_issue':
                    recommendations.push({
                        priority: 2,
                        title: '检查缓冲区管理',
                        detail: 'byte[] 占用大量内存，检查 I/O 流是否正确关闭，图片缓存是否合理'
                    });
                    break;
                case 'string_issue':
                    recommendations.push({
                        priority: 3,
                        title: '优化字符串处理',
                        detail: 'String 对象过多，使用 StringBuilder 替代字符串拼接，考虑 String.intern()'
                    });
                    break;
            }
        }
        
        return recommendations.sort((a, b) => a.priority - b.priority);
    }

    // ============================================
    // 工具函数
    // ============================================

    function getShortClassName(fullName) {
        if (!fullName) return '';
        const lastDot = fullName.lastIndexOf('.');
        return lastDot === -1 ? fullName : fullName.substring(lastDot + 1);
    }

    function isJDKClass(className) {
        return className.startsWith('java.') || 
               className.startsWith('javax.') ||
               className.startsWith('sun.') || 
               className.startsWith('jdk.') ||
               className.startsWith('com.sun.') ||
               className.includes('[]');
    }

    function isFrameworkClass(className) {
        const frameworks = [
            'org.springframework.', 'org.apache.', 'io.netty.',
            'com.google.', 'org.hibernate.', 'com.fasterxml.',
            'org.slf4j.', 'ch.qos.logback.'
        ];
        return frameworks.some(f => className.startsWith(f));
    }

    // ============================================
    // 渲染函数
    // ============================================

    /**
     * 渲染诊断概览
     */
    function render(data) {
        const container = document.getElementById('diagnosisContainer');
        if (!container) return;

        diagnosisData = runDiagnosis(data);

        // 渲染摘要卡片
        const summaryHtml = renderSummaryCard(diagnosisData);
        
        // 渲染问题列表
        const issuesHtml = renderIssuesList(diagnosisData.issues);
        
        // 渲染建议
        const recommendationsHtml = renderRecommendations(diagnosisData.recommendations);

        container.innerHTML = `
            ${summaryHtml}
            ${issuesHtml}
            ${recommendationsHtml}
        `;
    }

    /**
     * 渲染摘要卡片
     */
    function renderSummaryCard(diagnosis) {
        const summary = diagnosis.summary;
        const severityClass = diagnosis.severity;
        
        return `
            <div class="diagnosis-summary ${severityClass}">
                <div class="summary-icon">${summary.icon}</div>
                <div class="summary-content">
                    <div class="summary-title">${Utils.escapeHtml(summary.text)}</div>
                    <div class="summary-subtitle">${Utils.escapeHtml(summary.subtext)}</div>
                </div>
                <div class="summary-stats">
                    <div class="stat-item">
                        <span class="stat-value">${diagnosis.issues.filter(i => i.severity === 'critical').length}</span>
                        <span class="stat-label">严重</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value">${diagnosis.issues.filter(i => i.severity === 'warning').length}</span>
                        <span class="stat-label">警告</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value">${diagnosis.issues.filter(i => i.severity === 'info').length}</span>
                        <span class="stat-label">信息</span>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * 渲染问题列表
     */
    function renderIssuesList(issues) {
        if (issues.length === 0) {
            return `
                <div class="no-issues-message">
                    <div class="icon">✅</div>
                    <div class="title">未检测到明显问题</div>
                    <div class="hint">堆内存使用看起来正常，可以查看 Class Histogram 了解详情</div>
                </div>
            `;
        }

        // 只显示前 5 个最重要的问题
        const topIssues = issues.slice(0, 5);
        
        return `
            <div class="issues-section">
                <h3>🔍 检测到的问题</h3>
                <div class="issues-list">
                    ${topIssues.map((issue, idx) => renderIssueCard(issue, idx)).join('')}
                </div>
                ${issues.length > 5 ? `
                    <div class="more-issues-hint">
                        还有 ${issues.length - 5} 个问题，点击 "Root Cause" 标签查看完整分析
                    </div>
                ` : ''}
            </div>
        `;
    }

    /**
     * 渲染单个问题卡片
     */
    function renderIssueCard(issue, index) {
        const severityIcon = {
            critical: '🔴',
            warning: '🟡',
            info: '🔵'
        }[issue.severity];
        
        const severityLabel = {
            critical: '严重',
            warning: '警告',
            info: '信息'
        }[issue.severity];

        // 业务上下文信息（如果有）
        const businessContextHtml = issue.businessContext ? `
            <div class="issue-business-context">
                <div class="context-header">
                    <span class="context-icon">💡</span>
                    <span class="context-title">分析结论</span>
                </div>
                <div class="context-body">
                    ${issue.businessContext.framework ? `
                        <div class="context-item">
                            <span class="context-label">框架:</span>
                            <span class="context-value">${Utils.escapeHtml(issue.businessContext.framework)}</span>
                        </div>
                    ` : ''}
                    ${issue.businessContext.usage ? `
                        <div class="context-item">
                            <span class="context-label">用途:</span>
                            <span class="context-value">${Utils.escapeHtml(issue.businessContext.usage)}</span>
                        </div>
                    ` : ''}
                    ${issue.businessContext.businessClass ? `
                        <div class="context-item">
                            <span class="context-label">业务入口:</span>
                            <span class="context-value business-class">${Utils.escapeHtml(getShortClassName(issue.businessContext.businessClass))}</span>
                        </div>
                    ` : ''}
                    ${issue.businessContext.suggestion ? `
                        <div class="context-suggestion">
                            <span class="suggestion-icon">👉</span>
                            ${Utils.escapeHtml(issue.businessContext.suggestion)}
                        </div>
                    ` : ''}
                </div>
            </div>
        ` : '';

        const rootCauseHtml = issue.rootCause ? `
            <div class="issue-root-cause">
                <span class="cause-label">根因:</span>
                <span class="cause-detail">${Utils.escapeHtml(issue.rootCause.detail)}</span>
            </div>
        ` : '';

        // 只在没有业务上下文时显示原始持有者
        const retainersHtml = !issue.businessContext && issue.retainers && issue.retainers.length > 0 ? `
            <div class="issue-retainers">
                <span class="retainers-label">持有者:</span>
                ${issue.retainers.slice(0, 2).map(r => `
                    <span class="retainer-chip">
                        ${Utils.escapeHtml(getShortClassName(r.retainer_class))}.${Utils.escapeHtml(r.field_name || '?')}
                    </span>
                `).join('')}
                ${issue.retainers.length > 2 ? `<span class="more-retainers">+${issue.retainers.length - 2}</span>` : ''}
            </div>
        ` : '';

        return `
            <div class="issue-card ${issue.severity}" data-index="${index}">
                <div class="issue-header">
                    <span class="issue-severity">${severityIcon} ${severityLabel}</span>
                    <span class="issue-type">${getIssueTypeLabel(issue.type)}</span>
                </div>
                <div class="issue-title">${Utils.escapeHtml(issue.title)}</div>
                <div class="issue-description">${Utils.escapeHtml(issue.description)}</div>
                <div class="issue-metrics">
                    <span class="metric">📊 ${(issue.metrics.percentage || 0).toFixed(1)}%</span>
                    <span class="metric">💾 ${Utils.formatBytes(issue.metrics.size || 0)}</span>
                    <span class="metric">📦 ${Utils.formatNumber(issue.metrics.instanceCount || 0)} 实例</span>
                </div>
                ${businessContextHtml}
                ${rootCauseHtml}
                ${retainersHtml}
                <div class="issue-actions">
                    ${issue.actions.map(action => `
                        <button class="issue-action-btn" onclick="HeapDiagnosis.executeAction('${action.type}', '${Utils.escapeHtml(issue.className).replace(/'/g, "\\'")}')">
                            ${getActionIcon(action.type)} ${Utils.escapeHtml(action.label)}
                        </button>
                    `).join('')}
                </div>
            </div>
        `;
    }

    /**
     * 渲染建议
     */
    function renderRecommendations(recommendations) {
        if (recommendations.length === 0) return '';

        return `
            <div class="recommendations-section">
                <h3>💡 优化建议</h3>
                <div class="recommendations-list">
                    ${recommendations.map((rec, idx) => `
                        <div class="recommendation-card priority-${rec.priority}">
                            <div class="rec-priority">步骤 ${idx + 1}</div>
                            <div class="rec-content">
                                <div class="rec-title">${Utils.escapeHtml(rec.title)}</div>
                                <div class="rec-detail">${Utils.escapeHtml(rec.detail)}</div>
                            </div>
                        </div>
                    `).join('')}
                </div>
            </div>
        `;
    }

    function getIssueTypeLabel(type) {
        const labels = {
            'big_consumer': '大内存消费者',
            'leak_suspect': '泄漏嫌疑',
            'high_instance_count': '实例过多',
            'collection_issue': '集合问题',
            'byte_array_issue': '缓冲区问题',
            'string_issue': '字符串问题',
            'business_class': '业务类'
        };
        return labels[type] || type;
    }

    function getActionIcon(type) {
        const icons = {
            'search': '🔍',
            'view_retainers': '🔗',
            'check_lifecycle': '⏱️',
            'add_cleanup': '🧹',
            'check_creation': '🔨',
            'use_pool': '♻️',
            'check_io_buffers': '📁',
            'check_image_cache': '🖼️',
            'use_stringbuilder': '📝',
            'intern_strings': '🔤',
            'check_creation_point': '📍',
            // 新增的 action 类型
            'check_connections': '🔌',
            'check_request_queue': '📋',
            'tune_pool': '⚙️',
            'check_stream_close': '🚰',
            'check_file_size': '📄',
            'check_message_size': '📨',
            'check_batch_size': '📦',
            'check_cache_size': '💾',
            'check_cache_ttl': '⏰',
            'check_log_level': '📝',
            'check_async_log': '⚡',
            'check_image_size': '🖼️'
        };
        return icons[type] || '▶️';
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
     * 执行操作
     */
    function executeAction(actionType, className) {
        switch (actionType) {
            case 'search':
                if (typeof showPanel === 'function') {
                    showPanel('heaphistogram');
                }
                if (typeof HeapHistogram !== 'undefined') {
                    HeapHistogram.searchClass(className);
                }
                break;
            case 'view_retainers':
                if (typeof showPanel === 'function') {
                    showPanel('heapmergedpaths');
                }
                break;
            default:
                HeapCore.showNotification(`操作: ${actionType}`, 'info');
        }
    }

    /**
     * 获取诊断数据
     */
    function getDiagnosisData() {
        return diagnosisData;
    }

    // ============================================
    // 模块注册
    // ============================================

    const module = {
        init,
        render,
        executeAction,
        getDiagnosisData,
        runDiagnosis
    };

    // 自动注册到核心模块
    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('diagnosis', module);
    }

    return module;
})();

// 导出到全局
window.HeapDiagnosis = HeapDiagnosis;
