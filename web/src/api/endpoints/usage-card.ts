import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../client';
import { logger } from '@/lib/logger';

// ========== 类型定义 ==========

export type FieldSpec = {
    source: 'body' | 'header' | 'static';
    path: string;
    transform?: string[];
    optional?: boolean;
};

export type UsageMetricConfig = {
    id: string;
    label: string;
    kind: string;   // quota/counter/rate_limit/billing
    unit: string;   // requests/times/tokens/credits/usd/percent
    window: string; // 5h/weekly/monthly/hourly/minute/custom
    limit?: FieldSpec;
    used?: FieldSpec;
    remaining?: FieldSpec;
    reset_at?: FieldSpec;
};

export type UsageHeader = {
    key: string;
    value: string;
};

export type UsageCardConfig = {
    metrics?: UsageMetricConfig[];
};

export type UsageMetric = {
    id: string;
    label: string;
    kind: string;
    unit: string;
    window: string;
    limit?: number | null;
    used?: number | null;
    remaining?: number | null;
    percent?: number | null;
    reset_at?: string | null;
    status: string; // ok/warning/exhausted/unknown/error
    message?: string;
};

export type UsageSnapshot = {
    metrics?: UsageMetric[];
};

export type UsageCard = {
    id: number;
    name: string;
    template_id: string;
    account: string;
    endpoint: string;
    method: string;
    auth_type: string;
    auth_header: string;
    has_secret: boolean;
    extra_headers: UsageHeader[];
    config: UsageCardConfig;
    enabled: boolean;
    use_proxy: boolean;
    refresh_interval_sec: number;
    last_result: UsageSnapshot;
    last_error: string;
    last_refresh_at: string | null;
    created_at: string;
    updated_at: string;
};

export type UsageTemplate = {
    id: string;
    name: string;
    description: string;
    default_endpoint: string;
    method: string;
    auth_types: string[];
    required_headers: { key: string; value: string; placeholder?: string }[];
    metrics: {
        id: string;
        label: string;
        kind: string;
        unit: string;
        window: string;
        limit: FieldSpec;
        used?: FieldSpec;
        remaining?: FieldSpec;
        reset_at?: FieldSpec;
    }[];
    primary_metric_ids: string[];
};

export type CreateUsageCardRequest = {
    name: string;
    template_id: string;
    account?: string;
    endpoint?: string;
    method?: string;
    auth_type?: string;
    auth_header?: string;
    secret?: string;
    extra_headers?: UsageHeader[];
    config?: UsageCardConfig;
    enabled?: boolean;
    use_proxy?: boolean;
    refresh_interval_sec?: number;
};

export type UpdateUsageCardRequest = {
    id: number;
    name?: string;
    template_id?: string;
    account?: string;
    endpoint?: string;
    method?: string;
    auth_type?: string;
    auth_header?: string;
    secret?: string;
    extra_headers?: UsageHeader[];
    config?: UsageCardConfig;
    enabled?: boolean;
    use_proxy?: boolean;
    refresh_interval_sec?: number;
};

// ========== API Hooks ==========

export function useUsageCardTemplates() {
    return useQuery({
        queryKey: ['usage-card', 'templates'],
        queryFn: () => apiClient.get<UsageTemplate[]>('/api/v1/usage-card/templates'),
        staleTime: Infinity,
    });
}

export function useUsageCardList() {
    return useQuery({
        queryKey: ['usage-card', 'list'],
        queryFn: () => apiClient.get<UsageCard[]>('/api/v1/usage-card/list'),
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}

export function useCreateUsageCard() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: CreateUsageCardRequest) =>
            apiClient.post<UsageCard>('/api/v1/usage-card/create', data),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['usage-card', 'list'] });
        },
        onError: (error) => {
            logger.error('创建用量卡片失败:', error);
        },
    });
}

export function useUpdateUsageCard() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: UpdateUsageCardRequest) =>
            apiClient.post<UsageCard>('/api/v1/usage-card/update', data),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['usage-card', 'list'] });
        },
        onError: (error) => {
            logger.error('更新用量卡片失败:', error);
        },
    });
}

export function useDeleteUsageCard() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (id: number) =>
            apiClient.delete<null>(`/api/v1/usage-card/delete/${id}`),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['usage-card', 'list'] });
        },
        onError: (error) => {
            logger.error('删除用量卡片失败:', error);
        },
    });
}

export function useRefreshUsageCard() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (id: number) =>
            apiClient.post<UsageCard>(`/api/v1/usage-card/refresh/${id}`),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['usage-card', 'list'] });
        },
        onError: (error) => {
            logger.error('刷新用量卡片失败:', error);
        },
    });
}

export function useImportCodexChannelUsageCard() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: { channel_id: number; key_id: number }) =>
            apiClient.post<UsageCard>('/api/v1/usage-card/import/codex-channel', data),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['usage-card', 'list'] });
        },
        onError: (error) => {
            logger.error('导入 Codex 用量卡片失败:', error);
        },
    });
}

export function useBatchDeleteUsageCard() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (ids: number[]) =>
            apiClient.post<null>('/api/v1/usage-card/batch-delete', { ids }),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['usage-card', 'list'] });
        },
        onError: (error) => {
            logger.error('批量删除用量卡片失败:', error);
        },
    });
}

export function useBatchImportCodexChannel() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (items: { channel_id: number; key_id: number }[]) =>
            apiClient.post<unknown>('/api/v1/usage-card/batch-import/codex-channel', { items }),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['usage-card', 'list'] });
        },
        onError: (error) => {
            logger.error('批量导入 Codex 用量卡片失败:', error);
        },
    });
}
