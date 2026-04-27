'use client';

import { useState, useEffect } from 'react';
import { useAPIKeyList } from '@/api/endpoints/apikey';
import { useStatsAPIKeyDaily } from '@/api/endpoints/stats';
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts';
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { formatCount } from '@/lib/utils';
import dayjs from 'dayjs';
import { KeyRound } from 'lucide-react';
import { useTranslations } from 'next-intl';

export function KeyUsage() {
    const t = useTranslations('home.keyUsage');
    const { data: apiKeys } = useAPIKeyList();
    const [selectedKeyID, setSelectedKeyID] = useState<number | null>(null);
    const [days, setDays] = useState(7);

    // 默认选中第一个 key
    useEffect(() => {
        if (selectedKeyID === null && apiKeys && apiKeys.length > 0) {
            setSelectedKeyID(apiKeys[0].id);
        }
    }, [apiKeys, selectedKeyID]);

    const { data: dailyStats } = useStatsAPIKeyDaily(selectedKeyID, days);

    const chartData = (dailyStats ?? []).map((stat) => ({
        date: dayjs(stat.date).format('MM/DD'),
        tokens: stat.input_token + stat.output_token,
        requests: stat.request_success + stat.request_failed,
    }));

    const chartConfig = {
        tokens: { label: t('tokens') },
        requests: { label: t('requests') },
    };

    return (
        <div className="rounded-3xl bg-card border-card-border border p-4 text-card-foreground custom-shadow">
            <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2">
                    <KeyRound className="w-4 h-4 text-muted-foreground" />
                    <h3 className="font-semibold text-base">{t('title')}</h3>
                </div>
                <div className="flex items-center gap-2">
                    <Select value={selectedKeyID?.toString() ?? ''} onValueChange={(v) => setSelectedKeyID(Number(v))}>
                        <SelectTrigger className="w-32 h-8 text-xs">
                            <SelectValue placeholder={t('selectKey')} />
                        </SelectTrigger>
                        <SelectContent>
                            {apiKeys?.map((key) => (
                                <SelectItem key={key.id} value={key.id.toString()}>{key.name}</SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                    <Select value={days.toString()} onValueChange={(v) => setDays(Number(v))}>
                        <SelectTrigger className="w-20 h-8 text-xs">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                            <SelectItem value="7">{t('days', { count: 7 })}</SelectItem>
                            <SelectItem value="30">{t('days', { count: 30 })}</SelectItem>
                        </SelectContent>
                    </Select>
                </div>
            </div>

            {chartData.length > 0 ? (
                <ChartContainer config={chartConfig} className="h-32 w-full">
                    <AreaChart accessibilityLayer data={chartData}>
                        <defs>
                            <linearGradient id="fillKeyTokens" x1="0" y1="0" x2="0" y2="1">
                                <stop offset="5%" stopColor="var(--chart-3)" stopOpacity={0.8} />
                                <stop offset="95%" stopColor="var(--chart-3)" stopOpacity={0.1} />
                            </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" vertical={false} />
                        <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={10} />
                        <YAxis tickLine={false} axisLine={false} fontSize={10} tickFormatter={(v) => formatCount(v).formatted.value + formatCount(v).formatted.unit} />
                        <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="line" />} />
                        <Area type="monotone" dataKey="tokens" stroke="var(--chart-3)" fill="url(#fillKeyTokens)" />
                    </AreaChart>
                </ChartContainer>
            ) : (
                <div className="flex items-center justify-center h-32 text-sm text-muted-foreground">
                    {t('noData')}
                </div>
            )}
        </div>
    );
}
