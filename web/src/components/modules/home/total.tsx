'use client';

import { motion } from 'motion/react';
import {
    Activity,
    ChartColumnBig,
    Gauge,
    ArrowDown,
    ArrowUp,
    Minus,
} from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useStatsTotal, useStatsRealtime } from '@/api/endpoints/stats';
import { AnimatedNumber } from '@/components/common/AnimatedNumber';
import { EASING } from '@/lib/animations/fluid-transitions';
import { useMemo } from 'react';

type TrendDirection = 'up' | 'down' | 'neutral';

interface StatItem {
    label: string;
    value?: string;
    unit?: string;
    trend?: TrendDirection;
}

interface StatCard {
    title: string;
    icon: typeof Activity;
    accent: string;
    accentBg: string;
    items: StatItem[];
}

function TrendIcon({ direction }: { direction: TrendDirection }) {
    if (direction === 'up') return <ArrowUp className="size-3 text-emerald-500" />;
    if (direction === 'down') return <ArrowDown className="size-3 text-red-500" />;
    return <Minus className="size-3 text-muted-foreground/50" />;
}

export function Total() {
    const { data: statsTotalFormatted } = useStatsTotal();
    const { data: statsRealtime } = useStatsRealtime();
    const t = useTranslations('home.total');

    const cards: StatCard[] = useMemo(() => [
        {
            title: t('requestStats'),
            icon: Activity,
            accent: 'text-chart-2',
            accentBg: 'bg-chart-2/10',
            items: [
                {
                    label: t('requestCount'),
                    value: statsTotalFormatted?.request_count.formatted.value,
                    unit: statsTotalFormatted?.request_count.formatted.unit,
                },
                {
                    label: t('timeConsumed'),
                    value: statsTotalFormatted?.wait_time.formatted.value,
                    unit: statsTotalFormatted?.wait_time.formatted.unit,
                },
            ],
        },
        {
            title: t('totalStats'),
            icon: ChartColumnBig,
            accent: 'text-chart-1',
            accentBg: 'bg-chart-1/10',
            items: [
                {
                    label: t('totalToken'),
                    value: statsTotalFormatted?.total_token.formatted.value,
                    unit: statsTotalFormatted?.total_token.formatted.unit,
                },
                {
                    label: t('totalCost'),
                    value: statsTotalFormatted?.total_cost.formatted.value,
                    unit: statsTotalFormatted?.total_cost.formatted.unit,
                },
            ],
        },
        {
            title: t('realtimeStats'),
            icon: Gauge,
            accent: 'text-chart-5',
            accentBg: 'bg-chart-5/10',
            items: [
                {
                    label: t('rpm'),
                    value: ((statsRealtime?.rps ?? 0) * 60).toFixed(1),
                    unit: 'req/min',
                    trend: (statsRealtime?.rps ?? 0) > 0 ? 'up' : (statsRealtime?.rps ?? 0) < 0 ? 'down' : 'neutral',
                },
                {
                    label: t('tps'),
                    value: (statsRealtime?.tps ?? 0).toString(),
                    unit: 'tok/s',
                    trend: (statsRealtime?.tps ?? 0) > 0 ? 'up' : (statsRealtime?.tps ?? 0) < 0 ? 'down' : 'neutral',
                },
            ],
        },
    ], [statsTotalFormatted, statsRealtime, t]);

    return (
        <div className="grid grid-cols-3 gap-3">
            {cards.map((card, index) => (
                <motion.section
                    key={card.title}
                    className="rounded-2xl bg-card border border-border/50 overflow-hidden text-card-foreground"
                    initial={{ opacity: 0, y: 20, filter: 'blur(8px)' }}
                    animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
                    transition={{
                        duration: 0.5,
                        ease: EASING.easeOutExpo,
                        delay: index * 0.06,
                    }}
                >
                    <div className="p-4">
                        <div className="flex items-center gap-2 mb-3">
                            <div className={`size-7 rounded-lg ${card.accentBg} flex items-center justify-center shrink-0`}>
                                <card.icon className={`size-3.5 ${card.accent}`} />
                            </div>
                            <h3 className="font-medium text-sm text-muted-foreground truncate">{card.title}</h3>
                        </div>

                        <div className="space-y-2.5">
                            {card.items.map((item) => (
                                <div key={item.label} className="flex items-center justify-between">
                                    <span className="text-xs text-muted-foreground/70 truncate">{item.label}</span>
                                    <div className="flex items-baseline gap-0.5 shrink-0 tabular-nums">
                                        <span className="font-semibold text-lg leading-tight">
                                            <AnimatedNumber value={item.value} />
                                        </span>
                                        {item.unit && (
                                            <span className="text-xs text-muted-foreground/60">{item.unit}</span>
                                        )}
                                        {item.trend && <TrendIcon direction={item.trend} />}
                                    </div>
                                </div>
                            ))}
                        </div>
                    </div>
                </motion.section>
            ))}
        </div>
    );
}
