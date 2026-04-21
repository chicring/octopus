'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLogs } from '@/api/endpoints/log';
import { useAPIKeyList } from '@/api/endpoints/apikey';
import { useGroupList } from '@/api/endpoints/group';
import { LogCard } from './Item';
import { Loader2, ArrowUp, Wifi, WifiOff } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { VirtualizedGrid } from '@/components/common/VirtualizedGrid';
import { MultiSelect } from '@/components/common/MultiSelect';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

/**
 * 日志页面组件
 * - 初始加载 pageSize 条历史日志
 * - SSE 实时推送新日志
 * - 滚动自动加载更多
 * - 支持按 API Key 名称和模型名称筛选
 * - 支持筛选错误日志
 * - 支持断开/重连 SSE
 */
export function Log() {
    const t = useTranslations('log');
    const {
        logs,
        isConnected,
        hasMore,
        isLoading,
        isLoadingMore,
        loadMore,
        filterError,
        setFilterError,
        filterAPIKeyNames,
        setFilterAPIKeyNames,
        filterModelNames,
        setFilterModelNames,
        disconnect,
        reconnect,
    } = useLogs({ pageSize: 10 });

    const { data: apiKeys } = useAPIKeyList();
    const { data: groups } = useGroupList();

    const apiKeyOptions = useMemo(
        () => (apiKeys ?? []).map((k) => k.name).filter(Boolean),
        [apiKeys]
    );
    const modelOptions = useMemo(
        () => (groups ?? []).map((g) => g.name).filter(Boolean),
        [groups]
    );

    const scrollContainerRef = useRef<HTMLDivElement | null>(null);
    const [showScrollTop, setShowScrollTop] = useState(false);

    // 监听滚动位置
    useEffect(() => {
        const el = scrollContainerRef.current;
        if (!el) return;

        const handleScroll = () => {
            setShowScrollTop(el.scrollTop > 200);
        };

        el.addEventListener('scroll', handleScroll);
        return () => el.removeEventListener('scroll', handleScroll);
    }, []);

    // 滚动到顶部
    const scrollToTop = useCallback(() => {
        scrollContainerRef.current?.scrollTo({ top: 0, behavior: 'smooth' });
    }, []);

    const canLoadMore = hasMore && !isLoading && !isLoadingMore && logs.length > 0;
    const handleReachEnd = useCallback(() => {
        if (!canLoadMore) return;
        void loadMore();
    }, [canLoadMore, loadMore]);

    const footer = useMemo(() => {
        if (hasMore && (isLoading || isLoadingMore)) {
            return (
                <div className="flex justify-center py-4">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
            );
        }
        if (!hasMore && logs.length > 0) {
            return (
                <div className="flex justify-center py-4">
                    <span className="text-sm text-muted-foreground">{t('list.noMore')}</span>
                </div>
            );
        }
        return null;
    }, [hasMore, isLoading, isLoadingMore, logs.length, t]);

    return (
        <div className="flex flex-col h-full min-h-0">
            {/* 顶部控制栏 */}
            <div className="flex shrink-0 flex-wrap items-center gap-3 px-4 py-2">
                <MultiSelect
                    options={apiKeyOptions}
                    selected={filterAPIKeyNames}
                    onSelectedChange={setFilterAPIKeyNames}
                    placeholder={t('controls.filterAPIKey')}
                    searchPlaceholder={t('controls.searchAPIKey')}
                    emptyText={t('controls.noAPIKeys')}
                    selectAllText={t('controls.selectAll')}
                    deselectAllText={t('controls.deselectAll')}
                />
                <MultiSelect
                    options={modelOptions}
                    selected={filterModelNames}
                    onSelectedChange={setFilterModelNames}
                    placeholder={t('controls.filterModel')}
                    searchPlaceholder={t('controls.searchModel')}
                    emptyText={t('controls.noModels')}
                    selectAllText={t('controls.selectAll')}
                    deselectAllText={t('controls.deselectAll')}
                />
                <div className="flex-1" />
                <label className="flex min-h-11 items-center gap-2 text-sm">
                    <Switch
                        checked={filterError}
                        onCheckedChange={setFilterError}
                    />
                    <span className="text-muted-foreground">{t('controls.showErrorOnly')}</span>
                </label>
                <Button
                    variant="outline"
                    size="sm"
                    onClick={() => isConnected ? disconnect() : reconnect()}
                    className="min-h-11 min-w-11 gap-2 px-4"
                >
                    {isConnected ? (
                        <>
                            <Wifi className="h-4 w-4 text-green-500" />
                            <span>{t('controls.connected')}</span>
                        </>
                    ) : (
                        <>
                            <WifiOff className="h-4 w-4 text-muted-foreground" />
                            <span>{t('controls.disconnected')}</span>
                        </>
                    )}
                </Button>
            </div>

            {/* 日志列表 */}
            <div className="flex-1 min-h-0 relative">
                <VirtualizedGrid
                    items={logs}
                    layout="list"
                    columns={{ default: 1 }}
                    estimateItemHeight={80}
                    overscan={8}
                    getItemKey={(log) => `log-${log.id}`}
                    renderItem={(log) => <LogCard log={log} />}
                    footer={footer}
                    onReachEnd={handleReachEnd}
                    reachEndEnabled={canLoadMore}
                    reachEndOffset={2}
                    scrollContainerRef={scrollContainerRef}
                />

                {/* 滚动到顶部按钮 */}
                <button
                    onClick={scrollToTop}
                    className={cn(
                        "absolute bottom-4 right-4 z-10 p-2 rounded-full bg-card border shadow-lg transition-opacity",
                        showScrollTop ? "opacity-100" : "opacity-0 pointer-events-none"
                    )}
                    aria-label={t('controls.scrollToTop')}
                >
                    <ArrowUp className="h-5 w-5" />
                </button>
            </div>
        </div>
    );
}
