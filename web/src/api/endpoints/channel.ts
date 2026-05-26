import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../client';
import { logger } from '@/lib/logger';
import { formatCount, formatMoney, formatTime, formatRate } from '@/lib/utils';
import { StatsChannel, type StatsMetricsFormatted } from './stats';
/**
 * 渠道类型枚举
 */
export enum ChannelType {
    OpenAIChat = 0,
    OpenAIResponse = 1,
    Anthropic = 2,
    Gemini = 3,
    Volcengine = 4,
    OpenAIEmbedding = 5,
}

/**
 * 自动分组类型枚举
 */
export enum AutoGroupType {
    None = 0,   // 不自动分组
    Fuzzy = 1,  // 模糊匹配
    Exact = 2,  // 准确匹配
    Regex = 3,  // 正则匹配
}

export type BaseUrl = {
    url: string;
    delay: number;
};

export type CustomHeader = {
    header_key: string;
    header_value: string;
};

export type UsageQueryPreset = 'custom' | 'generic' | 'newapi' | 'tokenplan_official';

export type UsageQueryConfig = {
    enabled: boolean;
    preset: UsageQueryPreset;
    request_url: string;
    method: string;
    headers: CustomHeader[];
    timeout_sec: number;
    interval_min: number;
    api_key: string;
    access_token: string;
    user_id: string;
    template_code: string;
    extractor_code: string;
};

export type ChannelKey = {
    id: number;
    channel_id: number;
    enabled: boolean;
    channel_key: string;
    is_cli: boolean;
    multiplier: number;
    models: string;
    status_code: number;
    last_use_time_stamp: number;
    total_cost: number;
    total_requests: number;
    total_input_token: number;
    total_output_token: number;
    remark: string;
};

/**
 * 渠道完整数据（与后端 model.Channel 对齐；数组字段在前端保证为 []）
 */
export type Channel = {
    id: number;
    name: string;
    type: ChannelType;
    provider_id: string;
    official_url: string;
    enabled: boolean;
    base_urls: BaseUrl[];
    keys: ChannelKey[];
    model: string;
    custom_model: string;
    proxy: boolean;
    auto_sync: boolean;
    auto_group: AutoGroupType;
    custom_header: CustomHeader[];
    param_override?: string | null;
    channel_proxy?: string | null;
    usage_query: UsageQueryConfig;
    match_regex?: string | null;
    stats: StatsChannel;
};

// Internal type: backend may return null for slice fields; normalize to [] in select()
type ChannelServer = Omit<Channel, 'base_urls' | 'custom_header' | 'keys'> & {
    base_urls: BaseUrl[] | null;
    custom_header: CustomHeader[] | null;
    keys: ChannelKey[] | null;
    usage_query?: UsageQueryConfig | null;
};

export const DEFAULT_USAGE_QUERY: UsageQueryConfig = {
    enabled: false,
    preset: 'custom',
    request_url: '',
    method: 'GET',
    headers: [],
    timeout_sec: 30,
    interval_min: 0,
    api_key: '',
    access_token: '',
    user_id: '',
    template_code: '',
    extractor_code: '',
};

/**
 * 创建渠道请求：必填字段 + 可选字段
 */
export type CreateChannelRequest = {
    name: string;
    type: ChannelType;
    provider_id?: string;
    official_url?: string;
    enabled?: boolean;
    base_urls: BaseUrl[];
    keys: Array<Pick<ChannelKey, 'enabled' | 'channel_key' | 'is_cli' | 'multiplier' | 'models' | 'remark'>>;
    model: string;
    custom_model?: string;
    proxy?: boolean;
    auto_sync?: boolean;
    auto_group?: AutoGroupType;
    custom_header?: CustomHeader[];
    channel_proxy?: string | null;
    param_override?: string | null;
    usage_query?: UsageQueryConfig;
    match_regex?: string | null;
};

/**
 * 更新渠道请求：id + 可选字段 + keys diff
 */
export type UpdateChannelRequest = {
    id: number;
    provider_id?: string;
    name?: string;
    type?: ChannelType;
    official_url?: string;
    enabled?: boolean;
    base_urls?: BaseUrl[];
    model?: string;
    custom_model?: string;
    proxy?: boolean;
    auto_sync?: boolean;
    auto_group?: AutoGroupType;
    custom_header?: CustomHeader[];
    channel_proxy?: string | null;
    param_override?: string | null;
    usage_query?: UsageQueryConfig;
    match_regex?: string | null;
    // keys diff
    keys_to_add?: Array<Pick<ChannelKey, 'enabled' | 'channel_key' | 'is_cli' | 'multiplier' | 'models' | 'remark'>>;
    keys_to_update?: Array<{ id: number; enabled?: boolean; channel_key?: string; is_cli?: boolean; multiplier?: number; models?: string; remark?: string }>;
    keys_to_delete?: number[];
};

export type FetchModelRequest = {
    type: ChannelType;
    provider_id?: string;
    base_urls: BaseUrl[];
    keys: Array<Pick<ChannelKey, 'enabled' | 'channel_key' | 'is_cli' | 'multiplier' | 'models'>>;
    proxy?: boolean;
    channel_proxy?: string | null;
    match_regex?: string | null;
    custom_header?: CustomHeader[];
};

/**
 * 获取渠道列表 Hook
 * 
 * @example
 * const { data: channels, isLoading, error } = useChannelList();
 * 
 * if (isLoading) return <Loading />;
 * if (error) return <Error message={error.message} />;
 * 
 * channels?.forEach(channel => console.log(channel.raw.name));
 */
export function useChannelList() {
    return useQuery({
        queryKey: ['channels', 'list'],
        queryFn: async () => {
            return apiClient.get<ChannelServer[]>('/api/v1/channel/list');
        },
        select: (data) => data.map((item) => ({
            raw: ({
                ...item,
                official_url: item.official_url ?? '',
                base_urls: item.base_urls ?? [],
                custom_header: item.custom_header ?? [],
                keys: (item.keys ?? []).map((key) => ({
                    ...key,
                    is_cli: key.is_cli ?? false,
                    multiplier: key.multiplier && key.multiplier > 0 ? key.multiplier : 1,
                    models: key.models ?? '',
                })),
                usage_query: { ...DEFAULT_USAGE_QUERY, ...(item.usage_query ?? {}) },
            }) satisfies Channel,
            formatted: {
                input_token: formatCount(item.stats.input_token),
                output_token: formatCount(item.stats.output_token),
                total_token: formatCount(item.stats.input_token + item.stats.output_token),
                input_cost: formatMoney(item.stats.input_cost),
                output_cost: formatMoney(item.stats.output_cost),
                total_cost: formatMoney(item.stats.input_cost + item.stats.output_cost),
                request_success: formatCount(item.stats.request_success),
                request_failed: formatCount(item.stats.request_failed),
                request_count: formatCount(item.stats.request_success + item.stats.request_failed),
                wait_time: formatTime(item.stats.wait_time),
                output_time: formatTime(item.stats.output_time),
                output_tps: formatRate(item.stats.output_time > 0 ? (item.stats.output_token / item.stats.output_time * 1000) : 0),
            }
        })) as Array<{ raw: Channel; formatted: StatsMetricsFormatted }>,
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}

/**
 * 创建渠道 Hook
 * 
 * @example
 * const createChannel = useCreateChannel();
 * 
 * createChannel.mutate({
 *   name: 'OpenAI',
 *   type: ChannelType.OpenAIChat,
 *   base_urls: [{ url: 'https://api.openai.com', delay: 0 }],
 *   keys: [{ enabled: true, channel_key: 'sk-xxx' }],
 *   model: 'gpt-4',
 * });
 */
export function useCreateChannel() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (data: CreateChannelRequest) => {
            return apiClient.post<ChannelServer>('/api/v1/channel/create', data);
        },
        onSuccess: (data) => {
            logger.log('渠道创建成功:', data);
            queryClient.invalidateQueries({ queryKey: ['channels', 'list'] });
            queryClient.invalidateQueries({ queryKey: ['models', 'list'] });
            queryClient.invalidateQueries({ queryKey: ['models', 'channel'] });
        },
        onError: (error) => {
            logger.error('渠道创建失败:', error);
        },
    });
}

/**
 * 更新渠道 Hook
 * 
 * @example
 * const updateChannel = useUpdateChannel();
 * 
 * updateChannel.mutate({
 *   id: 1,
 *   name: 'OpenAI Updated',
 *   type: ChannelType.OpenAIChat,
 *   enabled: true,
 *   base_urls: [{ url: 'https://api.openai.com', delay: 0 }],
 *   keys_to_add: [{ enabled: true, channel_key: 'sk-xxx' }],
 *   model: 'gpt-4-turbo',
 *   proxy: false,
 * });
 */
export function useUpdateChannel() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (data: UpdateChannelRequest) => {
            return apiClient.post<ChannelServer>('/api/v1/channel/update', data);
        },
        onSuccess: (data) => {
            logger.log('渠道更新成功:', data);
            queryClient.invalidateQueries({ queryKey: ['channels', 'list'] });
            queryClient.invalidateQueries({ queryKey: ['models', 'channel'] });
        },
        onError: (error) => {
            logger.error('渠道更新失败:', error);
        },
    });
}

/**
 * 删除渠道 Hook
 * 
 * @example
 * const deleteChannel = useDeleteChannel();
 * 
 * deleteChannel.mutate(1); // 删除 ID 为 1 的渠道
 */
export function useDeleteChannel() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (id: number) => {
            return apiClient.delete<null>(`/api/v1/channel/delete/${id}`);
        },
        onSuccess: () => {
            logger.log('渠道删除成功');
            queryClient.invalidateQueries({ queryKey: ['channels', 'list'] });
            queryClient.invalidateQueries({ queryKey: ['models', 'channel'] });
        },
        onError: (error) => {
            logger.error('渠道删除失败:', error);
        },
    });
}

/**
 * 启用/禁用渠道 Hook
 * 
 * @example
 * const enableChannel = useEnableChannel();
 * 
 * enableChannel.mutate({ id: 1, enabled: true }); // 启用 ID 为 1 的渠道
 * enableChannel.mutate({ id: 1, enabled: false }); // 禁用 ID 为 1 的渠道
 */
export function useEnableChannel() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (data: { id: number; enabled: boolean }) => {
            return apiClient.post<null>('/api/v1/channel/enable', data);
        },
        onSuccess: () => {
            logger.log('渠道状态更新成功');
            queryClient.invalidateQueries({ queryKey: ['channels', 'list'] });
        },
        onError: (error) => {
            logger.error('渠道状态更新失败:', error);
        },
    });
}

/**
 * 获取渠道模型列表 Hook
 * 
 * @example
 * const fetchModel = useFetchModel();
 * 
 * fetchModel.mutate({
 *   type: ChannelType.OpenAIChat,
 *   base_urls: [{ url: 'https://api.openai.com', delay: 0 }],
 *   keys: [{ enabled: true, channel_key: 'sk-xxx' }],
 *   proxy: false,
 * });
 * 
 * // 在 onSuccess 中获取模型列表
 * fetchModel.data // ['gpt-4', 'gpt-3.5-turbo', ...]
 */
export function useFetchModel() {
    return useMutation({
        mutationFn: async (data: FetchModelRequest) => {
            return apiClient.post<string[]>('/api/v1/channel/fetch-model', data);
        },
        onSuccess: (data) => {
            logger.log('模型列表获取成功:', data);
        },
        onError: (error) => {
            logger.error('模型列表获取失败:', error);
        },
    });
}

/**
 * 获取渠道最后同步时间 Hook
 * 
 * @example
 * const lastSyncTime = useLastSyncTime();
 * 
 * if (lastSyncTime) {
 *   console.log('最后同步时间:', new Date(lastSyncTime).toLocaleString());
 * }
 */
export function useLastSyncTime() {
    return useQuery({
        queryKey: ['channels', 'last-sync-time'],
        queryFn: async () => {
            return apiClient.get<string>('/api/v1/channel/last-sync-time');
        },
        refetchInterval: 30000,
    });
}
/**
 * 同步渠道 Hook
 * 
 * @example
 * const syncChannel = useSyncChannel();
 * 
 * syncChannel.mutate();
 */
export function useSyncChannel() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async () => {
            return apiClient.post<null>('/api/v1/channel/sync');
        },
        onSuccess: () => {
            logger.log('渠道同步成功');
            queryClient.invalidateQueries({ queryKey: ['channels', 'last-sync-time'] });
        },
        onError: (error) => {
            logger.error('渠道同步失败:', error);
        },
    });
}

/**
 * 模型测试结果
 */
export type TestModelResult = {
    model: string;
    passed: boolean;
    error?: string;
    delay?: number;
};

/**
 * 测试已保存渠道的模型
 */
export function useTestChannelModels() {
    return useMutation({
        mutationFn: async (data: { channel_id: number; models: string[] }) => {
            return apiClient.post<TestModelResult[]>('/api/v1/channel/test-models', data);
        },
    });
}

/**
 * 测试未保存配置的模型（创建/编辑时使用）
 */
export function useTestChannelModelsByConfig() {
    return useMutation({
        mutationFn: async (data: FetchModelRequest & { models: string[] }) => {
            const { models, ...config } = data;
            return apiClient.post<TestModelResult[]>('/api/v1/channel/test-models-by-config', {
                ...config,
                model: models.join(','),
            });
        },
    });
}

/**
 * 使用指定 Key 测试渠道模型
 */
export function useTestChannelModelsByKey() {
    return useMutation({
        mutationFn: async (data: { channel_id: number; key_id: number; models: string[] }) => {
            return apiClient.post<TestModelResult[]>('/api/v1/channel/test-models-by-key', data);
        },
    });
}

/**
 * 获取 Codex 渠道列表（含 Key）用于用量卡片导入
 */
export function useCodexChannels() {
    return useQuery({
        queryKey: ['channels', 'codex-list'],
        queryFn: async () => {
            const channels = await apiClient.get<ChannelServer[]>('/api/v1/channel/list');
            return channels
                .filter(c => c.provider_id === 'codex' && c.keys && c.keys.length > 0)
                .map(item => ({
                    ...item,
                    base_urls: item.base_urls ?? [],
                    custom_header: item.custom_header ?? [],
                    keys: item.keys ?? [],
                })) as Channel[];
        },
    });
}

/**
 * Codex auth 文件导入结果项
 */
export type CodexAuthImportResultItem = {
    file: string;
    source: string;
    status: string;
    email?: string;
    account_id?: string;
    error?: string;
};

/**
 * Codex auth 文件导入批量结果
 */
export type CodexAuthImportBatchResult = {
    imported: number;
    updated: number;
    failed: number;
    skipped: number;
    results: CodexAuthImportResultItem[];
};

/**
 * Codex auth 文件导入 Hook（multipart/form-data）
 */
export function useCodexAuthImport() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async ({ channelId, files }: { channelId: number; files: File[] }) => {
            const formData = new FormData();
            for (const file of files) {
                formData.append('files', file);
            }
            // 使用 fetch 直接发送 FormData，不走 apiClient.post（它会 JSON.stringify）
            const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || '.';
            const url = `${API_BASE_URL}/api/v1/channel/${channelId}/codex/auth-files/import`;

            const headers = new Headers();
            // 不设置 Content-Type，让浏览器自动设置 multipart boundary
            // 添加 Authorization
            if (typeof window !== 'undefined') {
                const { useAuthStore } = await import('@/api/endpoints/user');
                const token = useAuthStore.getState().token;
                if (token) {
                    headers.set('Authorization', `Bearer ${token}`);
                }
            }

            const response = await fetch(url, {
                method: 'POST',
                headers,
                body: formData,
            });

            if (!response.ok) {
                const data = await response.json().catch(() => null);
                throw new Error(
                    (data && typeof data === 'object' && 'message' in data && typeof data.message === 'string')
                        ? data.message
                        : response.statusText
                );
            }

            const data = await response.json();
            // 标准 ApiResponse 格式
            if (data && typeof data === 'object' && 'data' in data) {
                return data.data as CodexAuthImportBatchResult;
            }
            return data as CodexAuthImportBatchResult;
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['channels', 'list'] });
        },
    });
}
