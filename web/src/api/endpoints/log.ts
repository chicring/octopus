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
    model_name: string;
    attempt_num: number;    // 第几次尝试
    status: AttemptStatus;
    duration: number;       // 耗时(毫秒)
    sticky?: boolean;
    msg?: string;
}

/**
 * 日志数据
 */
export interface RelayLog {
    id: number;
    time: number;                // 时间戳
    request_model_name: string;  // 请求模型名称
    request_api_key_name?: string; // 请求使用的 API Key 名称
    channel: number;             // 实际使用的渠道ID
    channel_name: string;        // 渠道名称
    actual_model_name: string;   // 实际使用模型名称
    input_tokens: number;        // 输入Token
    output_tokens: number;       // 输出Token
    ftut: number;                // 首字时间(毫秒)
    use_time: number;            // 总用时(毫秒)
    cost: number;                // 消耗费用
    request_content?: string;     // 请求内容（仅详情接口返回）
    response_content?: string;    // 响应内容（仅详情接口返回）
    error: string;               // 错误信息
    attempts?: ChannelAttempt[]; // 所有尝试记录
    total_attempts?: number;     // 总尝试次数
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
 * 
 * @example
 * const clearLogs = useClearLogs();
 * 
 * clearLogs.mutate();
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

const logsInfiniteQueryKey = (pageSize: number, filterError: boolean) => ['logs', 'infinite', pageSize, filterError] as const;

/**
 * 日志管理 Hook
 * 整合初始加载、SSE 实时推送、滚动加载更多
 *
 * @example
 * const { logs, isConnected, hasMore, isLoadingMore, loadMore, filterError, setFilterError, disconnect, reconnect } = useLogs();
 *
 * // logs 自动包含历史日志和实时日志，按时间倒序
 * logs.forEach(log => console.log(log.request_model_name));
 *
 * // 滚动到底部时加载更多
 * if (hasMore && !isLoadingMore) loadMore();
 *
 * // 筛选错误日志
 * setFilterError(true);
 *
 * // 断开/重连 SSE
 * disconnect();
 * reconnect();
 */
export function useLogs(options: { pageSize?: number } = {}) {
    const { pageSize = 20 } = options;

    const [filterError, setFilterError] = useState(false);
    const [isConnected, setIsConnected] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    // 使用 reconnectNonce 强制触发重连，而不是依赖 streamPaused 状态变化
    const [reconnectNonce, setReconnectNonce] = useState(0);
    const manualCloseRef = useRef(false);

    const eventSourceRef = useRef<EventSource | null>(null);
    const connectGenerationRef = useRef(0);

    const queryClient = useQueryClient();

    const logsQuery = useInfiniteQuery({
        queryKey: logsInfiniteQueryKey(pageSize, filterError),
        initialPageParam: 1,
        queryFn: async ({ pageParam }) => {
            const params = new URLSearchParams();
            params.set('page', String(pageParam));
            params.set('page_size', String(pageSize));
            if (filterError) {
                params.set('has_error', 'true');
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

    useEffect(() => {
        // 手动断开时不连接
        if (manualCloseRef.current) {
            return;
        }

        let cancelled = false;
        const currentGen = ++connectGenerationRef.current;

        const connect = async () => {
            try {
                const { token } = await apiClient.get<{ token: string }>('/api/v1/log/stream-token');
                if (cancelled || connectGenerationRef.current !== currentGen) return;

                const eventSource = new EventSource(`${API_BASE_URL}/api/v1/log/stream?token=${token}`);
                eventSourceRef.current = eventSource;

                eventSource.onopen = () => {
                    if (connectGenerationRef.current !== currentGen) {
                        eventSource.close();
                        return;
                    }
                    setIsConnected(true);
                    setError(null);
                };

                eventSource.onmessage = (event) => {
                    try {
                        const log: RelayLog = JSON.parse(event.data);
                        // SSE 推送时也应用 filterError 筛选
                        if (filterError && !log.error?.trim()) return;

                        queryClient.setQueryData(
                            logsInfiniteQueryKey(pageSize, filterError),
                            (old: InfiniteData<RelayLog[], number> | undefined) => {
                                if (!old) {
                                    return { pages: [[log]], pageParams: [1] };
                                }

                                const exists = old.pages.some((p) => p?.some((x) => x.id === log.id));
                                if (exists) return old;

                                const firstPage = old.pages[0] ?? [];
                                return { ...old, pages: [[log, ...firstPage], ...old.pages.slice(1)] };
                            }
                        );
                    } catch (e) {
                        logger.error('解析日志数据失败:', e);
                    }
                };

                eventSource.onerror = () => {
                    if (connectGenerationRef.current !== currentGen) return;
                    // 仅在非手动关闭时设置状态
                    if (!manualCloseRef.current) {
                        setIsConnected(false);
                        setError(new Error('SSE 连接断开'));
                    }
                    eventSource.close();
                    eventSourceRef.current = null;
                };
            } catch (e) {
                if (cancelled || connectGenerationRef.current !== currentGen) return;
                setError(e instanceof Error ? e : new Error('获取 stream token 失败'));
                logger.error('获取 stream token 失败:', e);
            }
        };

        connect();

        return () => {
            cancelled = true;
            eventSourceRef.current?.close();
            eventSourceRef.current = null;
        };
    }, [pageSize, queryClient, filterError, reconnectNonce]);

    const clear = useCallback(() => {
        queryClient.removeQueries({ queryKey: logsInfiniteQueryKey(pageSize, filterError) });
    }, [pageSize, filterError, queryClient]);

    const disconnect = useCallback(() => {
        manualCloseRef.current = true;
        eventSourceRef.current?.close();
        eventSourceRef.current = null;
        setIsConnected(false);
    }, []);

    const reconnect = useCallback(() => {
        manualCloseRef.current = false;
        setReconnectNonce((n) => n + 1);
    }, []);

    return {
        logs,
        isConnected,
        error,
        hasMore: !!logsQuery.hasNextPage,
        isLoading: logsQuery.isLoading,
        isLoadingMore: logsQuery.isFetchingNextPage,
        loadMore,
        clear,
        filterError,
        setFilterError,
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
