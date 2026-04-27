'use client';

import { useChannelList } from '@/api/endpoints/channel';
import { useStatsModel, useStatsAPIKey, type StatsModelFormatted, type StatsAPIKeyFormatted } from '@/api/endpoints/stats';
import { useAPIKeyList } from '@/api/endpoints/apikey';
import { useMemo } from 'react';
import { useTranslations } from 'next-intl';
import { TrendingUp } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger, TabsContents, TabsContent } from '@/components/animate-ui/components/animate/tabs';
import { useHomeViewStore, type RankSortMode, type RankDimension } from '@/components/modules/home/store';

type ChannelData = NonNullable<ReturnType<typeof useChannelList>['data']>[number];

export function Rank() {
    const { data: channelData } = useChannelList();
    const { data: modelData } = useStatsModel();
    const { data: statsAPIKeyData } = useStatsAPIKey();
    const { data: apiKeyList } = useAPIKeyList();
    const t = useTranslations('home.rank');
    const rankDimension = useHomeViewStore((state) => state.rankDimension);
    const setRankDimension = useHomeViewStore((state) => state.setRankDimension);
    const rankSortMode = useHomeViewStore((state) => state.rankSortMode);
    const setRankSortMode = useHomeViewStore((state) => state.setRankSortMode);

    const rankedChannelsByCost = useMemo<ChannelData[]>(() => {
        if (!channelData) return [];
        return [...channelData].sort((a, b) => b.formatted.total_cost.raw - a.formatted.total_cost.raw);
    }, [channelData]);

    const rankedChannelsByCount = useMemo<ChannelData[]>(() => {
        if (!channelData) return [];
        return [...channelData].sort((a, b) => b.formatted.request_count.raw - a.formatted.request_count.raw);
    }, [channelData]);

    const rankedChannelsByTokens = useMemo<ChannelData[]>(() => {
        if (!channelData) return [];
        return [...channelData].sort((a, b) => b.formatted.total_token.raw - a.formatted.total_token.raw);
    }, [channelData]);

    const rankedModelsByCost = useMemo<StatsModelFormatted[]>(() => {
        if (!modelData) return [];
        return [...modelData].sort((a, b) => b.total_cost.raw - a.total_cost.raw);
    }, [modelData]);

    const rankedModelsByCount = useMemo<StatsModelFormatted[]>(() => {
        if (!modelData) return [];
        return [...modelData].sort((a, b) => b.request_count.raw - a.request_count.raw);
    }, [modelData]);

    const rankedModelsByTokens = useMemo<StatsModelFormatted[]>(() => {
        if (!modelData) return [];
        return [...modelData].sort((a, b) => b.total_token.raw - a.total_token.raw);
    }, [modelData]);

    const rankedAPIKeysByCost = useMemo<StatsAPIKeyFormatted[]>(() => {
        if (!statsAPIKeyData) return [];
        return [...statsAPIKeyData].sort((a, b) => b.total_cost.raw - a.total_cost.raw);
    }, [statsAPIKeyData]);

    const rankedAPIKeysByCount = useMemo<StatsAPIKeyFormatted[]>(() => {
        if (!statsAPIKeyData) return [];
        return [...statsAPIKeyData].sort((a, b) => b.request_count.raw - a.request_count.raw);
    }, [statsAPIKeyData]);

    const rankedAPIKeysByTokens = useMemo<StatsAPIKeyFormatted[]>(() => {
        if (!statsAPIKeyData) return [];
        return [...statsAPIKeyData].sort((a, b) => b.total_token.raw - a.total_token.raw);
    }, [statsAPIKeyData]);

    const getMedalEmoji = (rank: number): string => {
        switch (rank) {
            case 1: return '🥇';
            case 2: return '🥈';
            case 3: return '🥉';
            default: return '';
        }
    };

    const renderChannelList = (channels: ChannelData[], mode: RankSortMode) => {
        if (channels.length === 0) {
            return (
                <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
                    <TrendingUp className="w-12 h-12 mb-3 opacity-30" />
                    <p className="text-sm">{t('noData')}</p>
                </div>
            );
        }
        return (
            <div className="space-y-3 max-h-[300px] overflow-y-auto">
                {channels.map((channel, index) => {
                    const rank = index + 1;
                    const medal = getMedalEmoji(rank);

                    return (
                        <div
                            key={channel.raw.id}
                            className="flex items-center gap-3 p-3 rounded-2xl hover:bg-accent/5 transition-colors"
                        >
                            <div className="w-8 h-8 rounded-lg flex items-center justify-center font-bold text-lg shrink-0">
                                {medal || rank}
                            </div>

                            <div className="flex-1 min-w-0">
                                <p className="font-medium text-sm truncate">{channel.raw.name}</p>
                                {mode === 'count' && (() => {
                                    const successCount = channel.formatted.request_success.raw;
                                    const failedCount = channel.formatted.request_failed.raw;
                                    const totalCount = successCount + failedCount;
                                    const successRate = totalCount > 0 ? (successCount / totalCount) * 100 : 0;

                                    return (
                                        <div className="flex items-center gap-1 text-xs text-muted-foreground mt-1">
                                            <span>{t('successRate')}:</span>
                                            <span>{successRate.toFixed(1)}%</span>
                                        </div>
                                    );
                                })()}
                            </div>

                            <div className="flex items-center gap-1 text-right shrink-0">
                                {mode === 'count' ? (
                                    <div className="flex items-center gap-1 text-sm font-medium tabular-nums">
                                        <span className="text-accent">
                                            {channel.formatted.request_success.formatted.value}
                                            <span className="text-xs text-muted-foreground">
                                                {channel.formatted.request_success.formatted.unit}
                                            </span>
                                        </span>
                                        <span className="text-muted-foreground/40 font-light">/</span>
                                        <span className="text-destructive">
                                            {channel.formatted.request_failed.formatted.value}
                                            <span className="text-xs text-muted-foreground">
                                                {channel.formatted.request_failed.formatted.unit}
                                            </span>
                                        </span>
                                    </div>
                                ) : mode === 'tokens' ? (
                                    <span className="font-semibold text-base">
                                        {channel.formatted.total_token.formatted.value}
                                        <span className="text-xs text-muted-foreground">
                                            {channel.formatted.total_token.formatted.unit}
                                        </span>
                                    </span>
                                ) : (
                                    <span className="font-semibold text-base">
                                        {channel.formatted.total_cost.formatted.value}
                                        <span className="text-xs text-muted-foreground">
                                            {channel.formatted.total_cost.formatted.unit}
                                        </span>
                                    </span>
                                )}
                            </div>
                        </div>
                    );
                })}
            </div>
        );
    };

    const renderModelList = (models: StatsModelFormatted[], mode: RankSortMode) => {
        if (models.length === 0) {
            return (
                <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
                    <TrendingUp className="w-12 h-12 mb-3 opacity-30" />
                    <p className="text-sm">{t('noData')}</p>
                </div>
            );
        }
        return (
            <div className="space-y-3 max-h-[300px] overflow-y-auto">
                {models.map((model, index) => {
                    const rank = index + 1;
                    const medal = getMedalEmoji(rank);

                    return (
                        <div
                            key={model.name}
                            className="flex items-center gap-3 p-3 rounded-2xl hover:bg-accent/5 transition-colors"
                        >
                            <div className="w-8 h-8 rounded-lg flex items-center justify-center font-bold text-lg shrink-0">
                                {medal || rank}
                            </div>

                            <div className="flex-1 min-w-0">
                                <p className="font-medium text-sm truncate">{model.name}</p>
                                {mode === 'count' && (() => {
                                    const successCount = model.request_success.raw;
                                    const failedCount = model.request_failed.raw;
                                    const totalCount = successCount + failedCount;
                                    const successRate = totalCount > 0 ? (successCount / totalCount) * 100 : 0;

                                    return (
                                        <div className="flex items-center gap-1 text-xs text-muted-foreground mt-1">
                                            <span>{t('successRate')}:</span>
                                            <span>{successRate.toFixed(1)}%</span>
                                        </div>
                                    );
                                })()}
                            </div>

                            <div className="flex items-center gap-1 text-right shrink-0">
                                {mode === 'count' ? (
                                    <div className="flex items-center gap-1 text-sm font-medium tabular-nums">
                                        <span className="text-accent">
                                            {model.request_success.formatted.value}
                                            <span className="text-xs text-muted-foreground">
                                                {model.request_success.formatted.unit}
                                            </span>
                                        </span>
                                        <span className="text-muted-foreground/40 font-light">/</span>
                                        <span className="text-destructive">
                                            {model.request_failed.formatted.value}
                                            <span className="text-xs text-muted-foreground">
                                                {model.request_failed.formatted.unit}
                                            </span>
                                        </span>
                                    </div>
                                ) : mode === 'tokens' ? (
                                    <span className="font-semibold text-base">
                                        {model.total_token.formatted.value}
                                        <span className="text-xs text-muted-foreground">
                                            {model.total_token.formatted.unit}
                                        </span>
                                    </span>
                                ) : (
                                    <span className="font-semibold text-base">
                                        {model.total_cost.formatted.value}
                                        <span className="text-xs text-muted-foreground">
                                            {model.total_cost.formatted.unit}
                                        </span>
                                    </span>
                                )}
                            </div>
                        </div>
                    );
                })}
            </div>
        );
    };

    const renderAPIKeyList = (apiKeys: StatsAPIKeyFormatted[], mode: RankSortMode) => {
        if (apiKeys.length === 0) {
            return (
                <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
                    <TrendingUp className="w-12 h-12 mb-3 opacity-30" />
                    <p className="text-sm">{t('noData')}</p>
                </div>
            );
        }
        return (
            <div className="space-y-3 max-h-[300px] overflow-y-auto">
                {apiKeys.map((apiKey, index) => {
                    const rank = index + 1;
                    const medal = getMedalEmoji(rank);
                    const keyName = apiKeyList?.find((k) => k.id === apiKey.api_key_id)?.name ?? `Key #${apiKey.api_key_id}`;

                    return (
                        <div
                            key={apiKey.api_key_id}
                            className="flex items-center gap-3 p-3 rounded-2xl hover:bg-accent/5 transition-colors"
                        >
                            <div className="w-8 h-8 rounded-lg flex items-center justify-center font-bold text-lg shrink-0">
                                {medal || rank}
                            </div>

                            <div className="flex-1 min-w-0">
                                <p className="font-medium text-sm truncate">{keyName}</p>
                                {mode === 'count' && (() => {
                                    const successCount = apiKey.request_success.raw;
                                    const failedCount = apiKey.request_failed.raw;
                                    const totalCount = successCount + failedCount;
                                    const successRate = totalCount > 0 ? (successCount / totalCount) * 100 : 0;

                                    return (
                                        <div className="flex items-center gap-1 text-xs text-muted-foreground mt-1">
                                            <span>{t('successRate')}:</span>
                                            <span>{successRate.toFixed(1)}%</span>
                                        </div>
                                    );
                                })()}
                            </div>

                            <div className="flex items-center gap-1 text-right shrink-0">
                                {mode === 'count' ? (
                                    <div className="flex items-center gap-1 text-sm font-medium tabular-nums">
                                        <span className="text-accent">
                                            {apiKey.request_success.formatted.value}
                                            <span className="text-xs text-muted-foreground">
                                                {apiKey.request_success.formatted.unit}
                                            </span>
                                        </span>
                                        <span className="text-muted-foreground/40 font-light">/</span>
                                        <span className="text-destructive">
                                            {apiKey.request_failed.formatted.value}
                                            <span className="text-xs text-muted-foreground">
                                                {apiKey.request_failed.formatted.unit}
                                            </span>
                                        </span>
                                    </div>
                                ) : mode === 'tokens' ? (
                                    <span className="font-semibold text-base">
                                        {apiKey.total_token.formatted.value}
                                        <span className="text-xs text-muted-foreground">
                                            {apiKey.total_token.formatted.unit}
                                        </span>
                                    </span>
                                ) : (
                                    <span className="font-semibold text-base">
                                        {apiKey.total_cost.formatted.value}
                                        <span className="text-xs text-muted-foreground">
                                            {apiKey.total_cost.formatted.unit}
                                        </span>
                                    </span>
                                )}
                            </div>
                        </div>
                    );
                })}
            </div>
        );
    };

    const getChannelList = (mode: RankSortMode) => {
        switch (mode) {
            case 'cost': return rankedChannelsByCost;
            case 'count': return rankedChannelsByCount;
            case 'tokens': return rankedChannelsByTokens;
        }
    };

    const getModelList = (mode: RankSortMode) => {
        switch (mode) {
            case 'cost': return rankedModelsByCost;
            case 'count': return rankedModelsByCount;
            case 'tokens': return rankedModelsByTokens;
        }
    };

    const getAPIKeyList = (mode: RankSortMode) => {
        switch (mode) {
            case 'cost': return rankedAPIKeysByCost;
            case 'count': return rankedAPIKeysByCount;
            case 'tokens': return rankedAPIKeysByTokens;
        }
    };

    return (
        <div className="rounded-3xl bg-card text-card-foreground border-card-border border p-4">
            <Tabs value={rankSortMode} onValueChange={(value) => setRankSortMode(value as RankSortMode)}>
                <div className="flex items-center justify-between mb-2">
                    <h3 className="font-semibold text-base">{t('title')}</h3>
                    <TabsList>
                        <TabsTrigger value="cost">{t('sortByCost')}</TabsTrigger>
                        <TabsTrigger value="count">{t('sortByCount')}</TabsTrigger>
                        <TabsTrigger value="tokens">{t('sortByTokens')}</TabsTrigger>
                    </TabsList>
                </div>
                <TabsContents>
                    <TabsContent value="cost">
                        {rankDimension === 'key'
                            ? renderAPIKeyList(getAPIKeyList('cost'), 'cost')
                            : rankDimension === 'channel'
                                ? renderChannelList(getChannelList('cost'), 'cost')
                                : renderModelList(getModelList('cost'), 'cost')}
                    </TabsContent>
                    <TabsContent value="count">
                        {rankDimension === 'key'
                            ? renderAPIKeyList(getAPIKeyList('count'), 'count')
                            : rankDimension === 'channel'
                                ? renderChannelList(getChannelList('count'), 'count')
                                : renderModelList(getModelList('count'), 'count')}
                    </TabsContent>
                    <TabsContent value="tokens">
                        {rankDimension === 'key'
                            ? renderAPIKeyList(getAPIKeyList('tokens'), 'tokens')
                            : rankDimension === 'channel'
                                ? renderChannelList(getChannelList('tokens'), 'tokens')
                                : renderModelList(getModelList('tokens'), 'tokens')}
                    </TabsContent>
                </TabsContents>
            </Tabs>
            <div className="flex justify-center mt-3 pt-2 border-t border-card-border/50">
                <Tabs value={rankDimension} onValueChange={(value) => setRankDimension(value as RankDimension)}>
                    <TabsList>
                        <TabsTrigger value="channel">{t('channelDimension')}</TabsTrigger>
                        <TabsTrigger value="model">{t('modelDimension')}</TabsTrigger>
                        <TabsTrigger value="key">{t('keyDimension')}</TabsTrigger>
                    </TabsList>
                </Tabs>
            </div>
        </div>
    );
}