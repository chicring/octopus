import type { InfiniteData } from '@tanstack/react-query';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiClient, API_BASE_URL } from '../client';
import { logger } from '@/lib/logger';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

/**
 * 尝试状态
 */
export type AttemptStatus = 'success' | 'failed' | 'circuit_break' | 'skipped';

/**
 * 单次渠道尝试信息
 */
export interface ChannelAttempt {
    channel_id: number;
    channel_key_id?: number;
    channel_name: string;
    channel_key_remark?: string;
    model_name: string;
    attempt_num: number;
    status: AttemptStatus;
    duration: number;
    sticky?: boolean;
    msg?: string;
}

/**
 * 日志数据
 */
export interface RelayLog {
    id: number;
    time: number;
    request_model_name: string;
    request_api_key_name?: string;
    channel: number;
    channel_name: string;
    actual_model_name: string;
    reasoning_effort?: string;
    input_tokens: number;
    output_tokens: number;
    cached_tokens: number;
    cache_creation_tokens: number;
    ftut: number;
    use_time: number;
    cost: number;
    request_content?: string;
    response_content?: string;
    debug_content?: string;
    error: string;
    attempts?: ChannelAttempt[];
    total_attempts?: number;
    user_agent?: string;
    client_name?: string;
}

/**
 * 活跃请求状态
 */
export type ActiveRequestStatus = 'forwarding' | 'waiting_first_token' | 'streaming' | 'processing';

/**
 * 活跃请求
 */
export interface ActiveRequest {
    id: number;
    start_time: number;
    request_model: string;
    api_key_name: string;
    status: ActiveRequestStatus;
    channel_name: string;
    attempt_count: number;
}

/**
 * 活跃请求 SSE 事件
 */
export interface ActiveRequestEvent {
    type: 'active_register' | 'active_update' | 'active_complete';
    request: ActiveRequest;
}

/**
 * 日志列表查询参数
 */
export interface LogListParams {
    page?: number;
    page_size?: number;
    start_time?: number;
    end_time?: number;
}

/**
 * 清空日志 Hook
 */
export function useClearLogs() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async () => {
            return apiClient.delete<null>('/api/v1/log/clear');
        },
        onSuccess: () => {
            logger.log('日志清空成功');
            queryClient.invalidateQueries({ queryKey: ['logs'] });
        },
        onError: (error) => {
            logger.error('日志清空失败:', error);
        },
    });
}

const logsInfiniteQueryKey = (
    pageSize: number,
    filterError: boolean,
    filterAPIKeyNames: string[],
    filterModelNames: string[]
) => ['logs', 'infinite', pageSize, filterError, filterAPIKeyNames, filterModelNames] as const;

// 模块级别的手动断开标志，持久化用户选择
function getManualDisconnectFlag(): boolean {
    if (typeof window === 'undefined') return false;
    return sessionStorage.getItem('log_stream_manual_disconnect') === 'true';
}

function setManualDisconnectFlag(value: boolean): void {
    if (typeof window === 'undefined') return;
    if (value) {
        sessionStorage.setItem('log_stream_manual_disconnect', 'true');
    } else {
        sessionStorage.removeItem('log_stream_manual_disconnect');
    }
}

/**
 * 日志管理 Hook
 * 整合初始加载、SSE 实时推送、滚动加载更多
 */
export function useLogs(options: { pageSize?: number } = {}) {
    const { pageSize = 20 } = options;

    const [filterError, setFilterError] = useState(false);
    const [filterAPIKeyNames, setFilterAPIKeyNames] = useState<string[]>([]);
    const [filterModelNames, setFilterModelNames] = useState<string[]>([]);
    const [isConnected, setIsConnected] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const [reconnectNonce, setReconnectNonce] = useState(0);
    const [activeRequests, setActiveRequests] = useState<ActiveRequest[]>([]);

    const eventSourceRef = useRef<EventSource | null>(null);
    const connectGenerationRef = useRef(0);

    // 使用 ref 保存筛选条件，避免 SSE 回调因状态变化而重新注册
    const filterAPIKeyNamesRef = useRef(filterAPIKeyNames);
    const filterModelNamesRef = useRef(filterModelNames);

    useEffect(() => { filterAPIKeyNamesRef.current = filterAPIKeyNames; }, [filterAPIKeyNames]);
    useEffect(() => { filterModelNamesRef.current = filterModelNames; }, [filterModelNames]);

    const queryClient = useQueryClient();

    const logsQuery = useInfiniteQuery({
        queryKey: logsInfiniteQueryKey(pageSize, filterError, filterAPIKeyNames, filterModelNames),
        initialPageParam: 1,
        queryFn: async ({ pageParam }) => {
            const params = new URLSearchParams();
            params.set('page', String(pageParam));
            params.set('page_size', String(pageSize));
            if (filterError) {
                params.set('has_error', 'true');
            }
            if (filterAPIKeyNames.length > 0) {
                params.set('api_key_names', filterAPIKeyNames.join(','));
            }
            if (filterModelNames.length > 0) {
                params.set('model_names', filterModelNames.join(','));
            }
            const result = await apiClient.get<RelayLog[] | null>(`/api/v1/log/list?${params.toString()}`);
            return result ?? [];
        },
        getNextPageParam: (lastPage, allPages) => {
            if (!lastPage || lastPage.length < pageSize) return undefined;
            return allPages.length + 1;
        },
        staleTime: Infinity,
        refetchOnMount: 'always',
    });

    const logs = useMemo(() => {
        const pages = logsQuery.data?.pages ?? [];
        const seen = new Set<number>();
        const merged: RelayLog[] = [];

        for (const page of pages) {
            for (const log of page) {
                if (seen.has(log.id)) continue;
                seen.add(log.id);
                merged.push(log);
            }
        }

        merged.sort((a, b) => b.time - a.time);
        return merged;
    }, [logsQuery.data]);

    const loadMore = useCallback(async () => {
        if (!logsQuery.hasNextPage) return;
        if (logsQuery.isFetchingNextPage) return;

        try {
            await logsQuery.fetchNextPage();
        } catch (e) {
            logger.error('加载更多日志失败:', e);
        }
    }, [logsQuery]);

    const closeEventSource = useCallback((target?: EventSource | null) => {
        const source = target ?? eventSourceRef.current;
        if (!source) return;
        source.onopen = null;
        source.onmessage = null;
        source.onerror = null;
        source.close();
        if (eventSourceRef.current === source) {
            eventSourceRef.current = null;
        }
    }, []);

    // 写入缓存的辅助函数
    const writeLogToCache = useCallback(
        (log: RelayLog, queryKey: readonly unknown[]) => {
            queryClient.setQueryData(
                queryKey,
                (old: InfiniteData<RelayLog[], number> | undefined) => {
                    if (!old) return { pages: [[log]], pageParams: [1] };
                    const exists = old.pages.some((p) => p?.some((x) => x.id === log.id));
                    if (exists) return old;
                    const firstPage = old.pages[0] ?? [];
                    return { ...old, pages: [[log, ...firstPage], ...old.pages.slice(1)] };
                }
            );
        },
        [queryClient]
    );

    useEffect(() => {
        if (getManualDisconnectFlag()) {
            return;
        }

        let cancelled = false;
        const currentGen = ++connectGenerationRef.current;

        const connect = async () => {
            try {
                const { token } = await apiClient.get<{ token: string }>('/api/v1/log/stream-token');
                if (cancelled || connectGenerationRef.current !== currentGen || getManualDisconnectFlag()) return;

                closeEventSource();
                const eventSource = new EventSource(`${API_BASE_URL}/api/v1/log/stream?token=${token}`);
                eventSourceRef.current = eventSource;

                eventSource.onopen = () => {
                    if (cancelled || connectGenerationRef.current !== currentGen || getManualDisconnectFlag()) {
                        closeEventSource(eventSource);
                        return;
                    }
                    // 重连时清空活跃请求列表，等待服务端推送最新快照
                    setActiveRequests([]);
                    setIsConnected(true);
                    setError(null);
                };

                eventSource.onmessage = (event) => {
                    if (cancelled || connectGenerationRef.current !== currentGen) return;

                    try {
                        const log: RelayLog = JSON.parse(event.data);

                        const akn = filterAPIKeyNamesRef.current;
                        const mn = filterModelNamesRef.current;

                        // 写入全量缓存（无筛选）
                        writeLogToCache(log, logsInfiniteQueryKey(pageSize, false, [], []));

                        // 如果有错误，也写入错误筛选缓存（无 API Key/模型筛选）
                        if (log.error?.trim()) {
                            writeLogToCache(log, logsInfiniteQueryKey(pageSize, true, [], []));
                        }

                        // 写入当前活跃筛选缓存
                        if (akn.length > 0 || mn.length > 0) {
                            const matchesAPIKey = akn.length === 0 || akn.includes(log.request_api_key_name ?? '');
                            const matchesModel = mn.length === 0 || mn.includes(log.request_model_name);

                            if (matchesAPIKey && matchesModel) {
                                writeLogToCache(log, logsInfiniteQueryKey(pageSize, false, akn, mn));
                                if (log.error?.trim()) {
                                    writeLogToCache(log, logsInfiniteQueryKey(pageSize, true, akn, mn));
                                }
                            }
                        }
                    } catch (e) {
                        logger.error('解析日志数据失败:', e);
                    }
                };

                // 监听活跃请求事件
                eventSource.addEventListener('active', (event) => {
                    if (cancelled || connectGenerationRef.current !== currentGen) return;

                    try {
                        const activeEvent: ActiveRequestEvent = JSON.parse(event.data);
                        setActiveRequests((prev) => {
                            switch (activeEvent.type) {
                                case 'active_register':
                                    return [activeEvent.request, ...prev.filter((r) => r.id !== activeEvent.request.id)];
                                case 'active_update':
                                    return prev.map((r) => r.id === activeEvent.request.id ? activeEvent.request : r);
                                case 'active_complete':
                                    return prev.filter((r) => r.id !== activeEvent.request.id);
                                default:
                                    return prev;
                            }
                        });
                    } catch (e) {
                        logger.error('解析活跃请求数据失败:', e);
                    }
                });

                eventSource.onerror = () => {
                    if (cancelled || connectGenerationRef.current !== currentGen) return;

                    const wasManualDisconnect = getManualDisconnectFlag();
                    setIsConnected(false);

                    if (!wasManualDisconnect) {
                        setError(new Error('SSE 连接断开'));
                    }

                    closeEventSource(eventSource);
                };
            } catch (e) {
                if (cancelled || connectGenerationRef.current !== currentGen) return;
                setIsConnected(false);
                setError(e instanceof Error ? e : new Error('获取 stream token 失败'));
                logger.error('获取 stream token 失败:', e);
            }
        };

        connect();

        return () => {
            cancelled = true;
            closeEventSource();
        };
    }, [closeEventSource, pageSize, queryClient, reconnectNonce, writeLogToCache]);

    const clear = useCallback(() => {
        queryClient.removeQueries({ queryKey: logsInfiniteQueryKey(pageSize, filterError, filterAPIKeyNames, filterModelNames) });
    }, [pageSize, filterError, filterAPIKeyNames, filterModelNames, queryClient]);

    const disconnect = useCallback(() => {
        setManualDisconnectFlag(true);
        closeEventSource();
        setIsConnected(false);
        setError(null);
    }, [closeEventSource]);

    const reconnect = useCallback(() => {
        setManualDisconnectFlag(false);
        setError(null);
        setIsConnected(false);
        closeEventSource();
        setReconnectNonce((n) => n + 1);
    }, [closeEventSource]);

    return {
        logs,
        isConnected,
        error,
        hasMore: !!logsQuery.hasNextPage,
        isLoading: logsQuery.isLoading,
        isLoadingMore: logsQuery.isFetchingNextPage,
        loadMore,
        clear,
        activeRequests,
        filterError,
        setFilterError,
        filterAPIKeyNames,
        setFilterAPIKeyNames,
        filterModelNames,
        setFilterModelNames,
        disconnect,
        reconnect,
    };
}

/**
 * 获取单条日志详情（含完整的 request_content 和 response_content）
 */
export async function getLogDetail(id: number): Promise<RelayLog> {
    return apiClient.get<RelayLog>(`/api/v1/log/${id}`);
}
