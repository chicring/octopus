'use client';

import { useStatsDaily, useStatsHourly, useStatsAPIKeyDaily, type StatsAPIKeyDaily, type StatsAPIKeyHourly } from '@/api/endpoints/stats';
import { useAPIKeyList } from '@/api/endpoints/apikey';
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart';
import { useMemo, useCallback } from 'react';
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts';
import { useTranslations } from 'next-intl';
import { formatCount, formatMoney, formatRate } from '@/lib/utils';
import dayjs from 'dayjs';
import { AnimatedNumber } from '@/components/common/AnimatedNumber';
import { Tabs, TabsList, TabsTrigger } from '@/components/animate-ui/components/animate/tabs';
import { useHomeViewStore, type ChartMetricType, type ChartPeriod } from '@/components/modules/home/store';
import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ChevronDown, KeyRound } from 'lucide-react';
import { useQueries } from '@tanstack/react-query';
import { apiClient } from '@/api/client';

interface DailyAgg {
    date: string;
    input_token: number;
    output_token: number;
    input_cost: number;
    output_cost: number;
    request_success: number;
    request_failed: number;
    output_time: number;
    wait_time: number;
}

interface HourlyAgg {
    hour: number;
    input_token: number;
    output_token: number;
    input_cost: number;
    output_cost: number;
    request_success: number;
    request_failed: number;
    output_time: number;
    wait_time: number;
}

function getMetricValue(type: ChartMetricType, raw: DailyAgg) {
    switch (type) {
        case 'cost': return raw.input_cost + raw.output_cost;
        case 'count': return raw.request_success + raw.request_failed;
        case 'tps': return raw.output_time > 0 ? (raw.output_token / raw.output_time * 1000) : 0;
        case 'tokens': return raw.input_token + raw.output_token;
    }
}

function getHourlyMetricValue(type: ChartMetricType, raw: HourlyAgg) {
    switch (type) {
        case 'cost': return raw.input_cost + raw.output_cost;
        case 'count': return raw.request_success + raw.request_failed;
        case 'tps': return raw.output_time > 0 ? (raw.output_token / raw.output_time * 1000) : 0;
        case 'tokens': return raw.input_token + raw.output_token;
    }
}

export function StatsChart() {
    const PERIODS: readonly ChartPeriod[] = ['1', '7', '30'];
    const { data: apiKeys } = useAPIKeyList();
    const t = useTranslations('home.chart');

    const chartMetricType = useHomeViewStore((state) => state.chartMetricType);
    const setChartMetricType = useHomeViewStore((state) => state.setChartMetricType);
    const period = useHomeViewStore((state) => state.chartPeriod);
    const setChartPeriod = useHomeViewStore((state) => state.setChartPeriod);
    const selectedKeyIDs = useHomeViewStore((state) => state.chartSelectedKeyIDs);
    const setSelectedKeyIDs = useHomeViewStore((state) => state.setChartSelectedKeyIDs);

    const { data: statsDaily } = useStatsDaily();
    const { data: statsHourly } = useStatsHourly();

    const hasKeyFilter = selectedKeyIDs.length > 0;
    const days = Number(period);

    // 批量查询选中密钥的 daily 数据（7天/30天视图）
    const keyDailyResults = useQueries({
        queries: selectedKeyIDs.map((id) => ({
            queryKey: ['stats', 'apikey-daily', id, days],
            queryFn: () => apiClient.get<StatsAPIKeyDaily[]>(`/api/v1/stats/apikey/${id}/daily?days=${days}`),
            enabled: hasKeyFilter && period !== '1',
            refetchInterval: 30000,
            refetchOnMount: 'always' as const,
        })),
    });

    // 批量查询选中密钥的 hourly 数据（今天视图）
    const keyHourlyResults = useQueries({
        queries: selectedKeyIDs.map((id) => ({
            queryKey: ['stats', 'apikey-hourly', id],
            queryFn: () => apiClient.get<StatsAPIKeyHourly[]>(`/api/v1/stats/apikey/${id}/hourly`),
            enabled: hasKeyFilter && period === '1',
            refetchInterval: 10000,
            refetchOnMount: 'always' as const,
        })),
    });

    // 聚合多个密钥的 daily 数据
    const aggregatedKeyDaily = useMemo((): DailyAgg[] | null => {
        if (!hasKeyFilter || period === '1') return null;
        const map = new Map<string, DailyAgg>();
        for (const result of keyDailyResults) {
            if (!result.data) continue;
            for (const d of result.data) {
                let existing = map.get(d.date);
                if (!existing) {
                    existing = { date: d.date, input_token: 0, output_token: 0, input_cost: 0, output_cost: 0, request_success: 0, request_failed: 0, output_time: 0, wait_time: 0 };
                    map.set(d.date, existing);
                }
                existing.input_token += d.input_token;
                existing.output_token += d.output_token;
                existing.input_cost += d.input_cost;
                existing.output_cost += d.output_cost;
                existing.request_success += d.request_success;
                existing.request_failed += d.request_failed;
                existing.output_time += d.output_time;
                existing.wait_time += d.wait_time;
            }
        }
        if (map.size === 0) return null;
        return Array.from(map.values())
            .sort((a, b) => a.date.localeCompare(b.date));
    }, [hasKeyFilter, period, keyDailyResults]);

    // 聚合多个密钥的 hourly 数据
    const aggregatedKeyHourly = useMemo((): HourlyAgg[] | null => {
        if (!hasKeyFilter || period !== '1') return null;
        const map = new Map<number, HourlyAgg>();
        for (const result of keyHourlyResults) {
            if (!result.data) continue;
            for (const d of result.data) {
                let existing = map.get(d.hour);
                if (!existing) {
                    existing = { hour: d.hour, input_token: 0, output_token: 0, input_cost: 0, output_cost: 0, request_success: 0, request_failed: 0, output_time: 0, wait_time: 0 };
                    map.set(d.hour, existing);
                }
                existing.input_token += d.input_token;
                existing.output_token += d.output_token;
                existing.input_cost += d.input_cost;
                existing.output_cost += d.output_cost;
                existing.request_success += d.request_success;
                existing.request_failed += d.request_failed;
                existing.output_time += d.output_time;
                existing.wait_time += d.wait_time;
            }
        }
        if (map.size === 0) return null;
        return Array.from(map.values())
            .sort((a, b) => a.hour - b.hour);
    }, [hasKeyFilter, period, keyHourlyResults]);

    const sortedDaily = useMemo(() => {
        if (!statsDaily) return [];
        return [...statsDaily].sort((a, b) => a.date.localeCompare(b.date));
    }, [statsDaily]);

    const getChartDataKey = (type: ChartMetricType) => {
        return type === 'cost' ? 'total_cost' : type === 'count' ? 'request_count' : type === 'tokens' ? 'total_token' : 'output_tps';
    };

    const chartData = useMemo(() => {
        const dataKey = getChartDataKey(chartMetricType);
        // 有密钥筛选 + 非"今天"视图：使用聚合后的 daily 数据
        if (hasKeyFilter && period !== '1' && aggregatedKeyDaily) {
            return aggregatedKeyDaily.slice(-days).map((stat) => ({
                date: dayjs(stat.date).format('MM/DD'),
                [dataKey]: getMetricValue(chartMetricType, stat),
            }));
        }
        // "今天"视图
        if (period === '1') {
            // 有密钥筛选：使用聚合后的 hourly 数据
            if (hasKeyFilter && aggregatedKeyHourly) {
                return aggregatedKeyHourly.map((stat) => ({
                    date: `${stat.hour}:00`,
                    [dataKey]: getHourlyMetricValue(chartMetricType, stat),
                }));
            }
            // 无密钥筛选：使用全局 hourly 数据
            if (!statsHourly) return [];
            return statsHourly.map((stat) => ({
                date: `${stat.hour}:00`,
                [dataKey]: chartMetricType === 'cost'
                    ? stat.total_cost.raw
                    : chartMetricType === 'count'
                        ? stat.request_count.raw
                        : chartMetricType === 'tps'
                            ? (stat.output_time.raw > 0 ? (stat.output_token.raw / stat.output_time.raw * 1000) : 0)
                            : (stat.input_token.raw + stat.output_token.raw),
            }));
        } else {
            // 无密钥筛选 + 非"今天"视图
            return sortedDaily.slice(-days).map((stat) => ({
                date: dayjs(stat.date).format('MM/DD'),
                [dataKey]: chartMetricType === 'cost'
                    ? stat.total_cost.raw
                    : chartMetricType === 'count'
                        ? (stat.request_success.raw + stat.request_failed.raw)
                        : chartMetricType === 'tps'
                            ? (stat.output_time.raw > 0 ? (stat.output_token.raw / stat.output_time.raw * 1000) : 0)
                            : (stat.input_token.raw + stat.output_token.raw),
            }));
        }
    }, [sortedDaily, statsHourly, period, chartMetricType, hasKeyFilter, aggregatedKeyDaily, aggregatedKeyHourly, days]);

    const totals = useMemo(() => {
        // 有密钥筛选 + 非"今天"视图
        if (hasKeyFilter && period !== '1' && aggregatedKeyDaily) {
            const recentStats = aggregatedKeyDaily.slice(-days);
            const requests = recentStats.reduce((acc, s) => acc + s.request_success + s.request_failed, 0);
            const cost = recentStats.reduce((acc, s) => acc + s.input_cost + s.output_cost, 0);
            const tokens = recentStats.reduce((acc, s) => acc + s.input_token + s.output_token, 0);
            const totalOutputToken = recentStats.reduce((acc, s) => acc + s.output_token, 0);
            const totalOutputTime = recentStats.reduce((acc, s) => acc + s.output_time, 0);
            const tps = totalOutputTime > 0 ? (totalOutputToken / totalOutputTime * 1000) : 0;
            return { requests, cost, tokens, tps };
        }
        // "今天"视图
        if (period === '1') {
            // 有密钥筛选
            if (hasKeyFilter && aggregatedKeyHourly) {
                const requests = aggregatedKeyHourly.reduce((acc, s) => acc + s.request_success + s.request_failed, 0);
                const cost = aggregatedKeyHourly.reduce((acc, s) => acc + s.input_cost + s.output_cost, 0);
                const tokens = aggregatedKeyHourly.reduce((acc, s) => acc + s.input_token + s.output_token, 0);
                const totalOutputToken = aggregatedKeyHourly.reduce((acc, s) => acc + s.output_token, 0);
                const totalOutputTime = aggregatedKeyHourly.reduce((acc, s) => acc + s.output_time, 0);
                const tps = totalOutputTime > 0 ? (totalOutputToken / totalOutputTime * 1000) : 0;
                return { requests, cost, tokens, tps };
            }
            // 无密钥筛选
            if (!statsHourly) return { requests: 0, cost: 0, tokens: 0, tps: 0 };
            const requests = statsHourly.reduce((acc, stat) => acc + stat.request_count.raw, 0);
            const cost = statsHourly.reduce((acc, stat) => acc + stat.total_cost.raw, 0);
            const tokens = statsHourly.reduce((acc, stat) => acc + stat.input_token.raw + stat.output_token.raw, 0);
            const totalOutputToken = statsHourly.reduce((acc, stat) => acc + stat.output_token.raw, 0);
            const totalOutputTime = statsHourly.reduce((acc, stat) => acc + stat.output_time.raw, 0);
            const tps = totalOutputTime > 0 ? (totalOutputToken / totalOutputTime * 1000) : 0;
            return { requests, cost, tokens, tps };
        } else {
            // 无密钥筛选 + 非"今天"视图
            const recentStats = sortedDaily.slice(-days);
            const requests = recentStats.reduce((acc, stat) => acc + stat.request_success.raw + stat.request_failed.raw, 0);
            const cost = recentStats.reduce((acc, stat) => acc + stat.total_cost.raw, 0);
            const tokens = recentStats.reduce((acc, stat) => acc + stat.input_token.raw + stat.output_token.raw, 0);
            const totalOutputToken = recentStats.reduce((acc, stat) => acc + stat.output_token.raw, 0);
            const totalOutputTime = recentStats.reduce((acc, stat) => acc + stat.output_time.raw, 0);
            const tps = totalOutputTime > 0 ? (totalOutputToken / totalOutputTime * 1000) : 0;
            return { requests, cost, tokens, tps };
        }
    }, [sortedDaily, statsHourly, period, hasKeyFilter, aggregatedKeyDaily, aggregatedKeyHourly, days]);

    const chartConfig = useMemo(() => {
        const dataKey = getChartDataKey(chartMetricType);
        const labels = {
            'total_cost': t('totalCost'),
            'request_count': t('totalRequests'),
            'total_token': t('totalTokens'),
            'output_tps': t('totalTps'),
        };
        return {
            [dataKey]: { label: labels[dataKey] },
        };
    }, [chartMetricType, t]);

    const getPeriodLabel = (p: ChartPeriod) => {
        const labels = {
            '1': t('period.today'),
            '7': t('period.last7Days'),
            '30': t('period.last30Days'),
        };
        return labels[p];
    };

    const handlePeriodClick = () => {
        const currentIndex = PERIODS.indexOf(period);
        const nextIndex = (currentIndex + 1) % PERIODS.length;
        setChartPeriod(PERIODS[nextIndex]);
    };

    const getChartStroke = (type: ChartMetricType) => {
        if (type === 'cost') return 'var(--chart-1)';
        if (type === 'count') return 'var(--chart-2)';
        if (type === 'tokens') return 'var(--chart-3)';
        return 'var(--chart-4)';
    };

    const getChartFill = (type: ChartMetricType) => {
        if (type === 'cost') return 'url(#fillMetric1)';
        if (type === 'count') return 'url(#fillMetric2)';
        if (type === 'tokens') return 'url(#fillMetric3)';
        return 'url(#fillMetric4)';
    };

    const toggleKey = useCallback((id: number) => {
        setSelectedKeyIDs(
            selectedKeyIDs.includes(id)
                ? selectedKeyIDs.filter((k) => k !== id)
                : [...selectedKeyIDs, id]
        );
    }, [selectedKeyIDs, setSelectedKeyIDs]);

    const clearKeys = useCallback(() => {
        setSelectedKeyIDs([]);
    }, [setSelectedKeyIDs]);

    const keyFilterLabel = useMemo(() => {
        if (selectedKeyIDs.length === 0) return t('allKeys');
        if (selectedKeyIDs.length === 1) {
            const key = apiKeys?.find((k) => k.id === selectedKeyIDs[0]);
            return key?.name ?? `Key #${selectedKeyIDs[0]}`;
        }
        return t('selectedCount', { count: selectedKeyIDs.length });
    }, [selectedKeyIDs, apiKeys, t]);

    return (
        <div className="rounded-3xl bg-card border-card-border border pt-4 pb-0 text-card-foreground custom-shadow">
            <div className="px-4 pb-2 space-y-2">
                <div className="flex justify-between items-center">
                    <div className="flex items-center gap-2">
                        <h3 className="font-semibold text-base">{t('title')}</h3>
                        {/* 密钥筛选下拉 */}
                        <Popover>
                            <PopoverTrigger asChild>
                                <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground">
                                    <KeyRound className="size-3" />
                                    <span className="max-w-24 truncate">{keyFilterLabel}</span>
                                    <ChevronDown className="size-3 opacity-50" />
                                </Button>
                            </PopoverTrigger>
                            <PopoverContent align="start" className="w-56 p-2 max-h-64 overflow-y-auto">
                                <div className="space-y-1">
                                    <div
                                        className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent cursor-pointer"
                                        onClick={clearKeys}
                                    >
                                        <Checkbox checked={selectedKeyIDs.length === 0} />
                                        <span>{t('allKeys')}</span>
                                    </div>
                                    {apiKeys?.map((key) => (
                                        <div
                                            key={key.id}
                                            className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent cursor-pointer"
                                            onClick={() => toggleKey(key.id)}
                                        >
                                            <Checkbox checked={selectedKeyIDs.includes(key.id)} />
                                            <span className="truncate">{key.name}</span>
                                        </div>
                                    ))}
                                </div>
                                {selectedKeyIDs.length > 0 && (
                                    <div className="border-t mt-1 pt-1">
                                        <div
                                            className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground cursor-pointer"
                                            onClick={clearKeys}
                                        >
                                            {t('clearFilter')}
                                        </div>
                                    </div>
                                )}
                            </PopoverContent>
                        </Popover>
                        {selectedKeyIDs.length > 0 && (
                            <Badge variant="secondary" className="text-[10px] h-5 cursor-pointer" onClick={clearKeys}>
                                {selectedKeyIDs.length} ×
                            </Badge>
                        )}
                    </div>
                    <Tabs value={chartMetricType} onValueChange={(value) => setChartMetricType(value as ChartMetricType)}>
                        <TabsList>
                            <TabsTrigger value="cost">{t('metricType.cost')}</TabsTrigger>
                            <TabsTrigger value="count">{t('metricType.count')}</TabsTrigger>
                            <TabsTrigger value="tokens">{t('metricType.tokens')}</TabsTrigger>
                            <TabsTrigger value="tps">{t('metricType.tps')}</TabsTrigger>
                        </TabsList>
                    </Tabs>
                </div>

                {/* 汇总统计 + 周期选择 */}
                <div className="flex justify-between items-start">
                    <div className="flex gap-2 text-sm">
                        <div>
                            <div className="text-xs text-muted-foreground">{t('totalRequests')}</div>
                            <div className="text-xl font-semibold">
                                <AnimatedNumber value={formatCount(totals.requests).formatted.value} />
                                <span className="ml-0.5 text-sm text-muted-foreground">{formatCount(totals.requests).formatted.unit}</span>
                            </div>
                        </div>
                        <div className="w-px bg-border self-stretch"></div>
                        <div>
                            <div className="text-xs text-muted-foreground">{t('totalCost')}</div>
                            <div className="text-xl font-semibold">
                                <AnimatedNumber value={formatMoney(totals.cost).formatted.value} />
                                <span className="ml-0.5 text-sm text-muted-foreground">{formatMoney(totals.cost).formatted.unit}</span>
                            </div>
                        </div>
                        <div className="w-px bg-border self-stretch"></div>
                        <div>
                            <div className="text-xs text-muted-foreground">{t('totalTokens')}</div>
                            <div className="text-xl font-semibold">
                                <AnimatedNumber value={formatCount(totals.tokens).formatted.value} />
                                <span className="ml-0.5 text-sm text-muted-foreground">{formatCount(totals.tokens).formatted.unit}</span>
                            </div>
                        </div>
                        <div className="w-px bg-border self-stretch"></div>
                        <div>
                            <div className="text-xs text-muted-foreground">{t('totalTps')}</div>
                            <div className="text-xl font-semibold">
                                <AnimatedNumber value={formatRate(totals.tps).formatted.value} />
                                <span className="ml-0.5 text-sm text-muted-foreground">{formatRate(totals.tps).formatted.unit}</span>
                            </div>
                        </div>
                    </div>
                    <div
                        className="flex gap-2 text-sm cursor-pointer hover:opacity-80 transition-opacity"
                        onClick={handlePeriodClick}
                    >
                        <div>
                            <div className="text-xs text-muted-foreground">{t('timePeriod')}</div>
                            <div className="text-base font-semibold">{getPeriodLabel(period)}</div>
                        </div>
                    </div>
                </div>
            </div>
            <ChartContainer config={chartConfig} className="h-40 w-full" >
                <AreaChart accessibilityLayer data={chartData}>
                    <defs>
                        <linearGradient id="fillMetric1" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--chart-1)" stopOpacity={1.0} />
                            <stop offset="95%" stopColor="var(--chart-1)" stopOpacity={0.1} />
                        </linearGradient>
                        <linearGradient id="fillMetric2" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--chart-2)" stopOpacity={1.0} />
                            <stop offset="95%" stopColor="var(--chart-2)" stopOpacity={0.1} />
                        </linearGradient>
                        <linearGradient id="fillMetric3" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--chart-3)" stopOpacity={1.0} />
                            <stop offset="95%" stopColor="var(--chart-3)" stopOpacity={0.1} />
                        </linearGradient>
                        <linearGradient id="fillMetric4" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--chart-4)" stopOpacity={1.0} />
                            <stop offset="95%" stopColor="var(--chart-4)" stopOpacity={0.1} />
                        </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} />
                    <XAxis dataKey="date" tickLine={false} axisLine={false} />
                    <YAxis
                        tickLine={false}
                        axisLine={false}
                        tickFormatter={(value) => {
                            if (chartMetricType === 'cost') {
                                const formatted = formatMoney(value);
                                return `${formatted.formatted.value}${formatted.formatted.unit}`;
                            } else if (chartMetricType === 'tps') {
                                const formatted = formatRate(value);
                                return `${formatted.formatted.value}${formatted.formatted.unit}`;
                            } else {
                                const formatted = formatCount(value);
                                return `${formatted.formatted.value}${formatted.formatted.unit}`;
                            }
                        }}
                    />
                    <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="line" />} />
                    <Area
                        type="monotone"
                        dataKey={getChartDataKey(chartMetricType)}
                        stroke={getChartStroke(chartMetricType)}
                        fill={getChartFill(chartMetricType)}
                    />
                </AreaChart>
            </ChartContainer>
        </div>
    );
}
