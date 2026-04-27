'use client';

import { motion } from 'motion/react';
import {
    Activity,
    ArrowDownToLine,
    ChartColumnBig,
    ArrowUpFromLine,
    Gauge,
} from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useStatsTotal, useStatsRealtime } from '@/api/endpoints/stats';
import { AnimatedNumber } from '@/components/common/AnimatedNumber';
import { EASING } from '@/lib/animations/fluid-transitions';


export function Total() {
    const { data: statsTotalFormatted } = useStatsTotal();
    const { data: statsRealtime } = useStatsRealtime();
    const t = useTranslations('home.total');

    const cards = [
        {
            title: t('requestStats'),
            headerIcon: Activity,
            accent: 'bg-primary',
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
            headerIcon: ChartColumnBig,
            accent: 'bg-chart-1',
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
            title: t('inputStats'),
            headerIcon: ArrowDownToLine,
            accent: 'bg-chart-3',
            items: [
                {
                    label: t('inputTokens'),
                    value: statsTotalFormatted?.input_token.formatted.value,
                    unit: statsTotalFormatted?.input_token.formatted.unit,
                },
                {
                    label: t('inputCost'),
                    value: statsTotalFormatted?.input_cost.formatted.value,
                    unit: statsTotalFormatted?.input_cost.formatted.unit,
                },
            ],
        },
        {
            title: t('outputStats'),
            headerIcon: ArrowUpFromLine,
            accent: 'bg-chart-4',
            items: [
                {
                    label: t('outputTokens'),
                    value: statsTotalFormatted?.output_token.formatted.value,
                    unit: statsTotalFormatted?.output_token.formatted.unit,
                },
                {
                    label: t('outputCost'),
                    value: statsTotalFormatted?.output_cost.formatted.value,
                    unit: statsTotalFormatted?.output_cost.formatted.unit,
                },
            ],
        },
        {
            title: t('realtimeStats'),
            headerIcon: Gauge,
            accent: 'bg-chart-5',
            items: [
                {
                    label: t('rps'),
                    value: (statsRealtime?.rps ?? 0).toString(),
                    unit: 'req/s',
                },
                {
                    label: t('tps'),
                    value: (statsRealtime?.tps ?? 0).toString(),
                    unit: 'tok/s',
                },
            ],
        },
    ];

    return (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
            {cards.map((card, index) => (
                <motion.section
                    key={card.title}
                    className="rounded-2xl bg-card border border-border/50 overflow-hidden text-card-foreground flex flex-col"
                    initial={{ opacity: 0, y: 20, filter: 'blur(8px)' }}
                    animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
                    transition={{
                        duration: 0.5,
                        ease: EASING.easeOutExpo,
                        delay: index * 0.06,
                    }}
                >
                    <div className={`h-0.5 ${card.accent}`} />

                    <div className="flex flex-col gap-3 p-4">
                        <div className="flex items-center gap-1.5">
                            <card.headerIcon className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                            <h3 className="font-medium text-xs text-muted-foreground truncate">{card.title}</h3>
                        </div>

                        <div className="flex flex-col gap-2">
                            {card.items.map((item) => (
                                <div key={item.label} className="flex flex-col">
                                    <span className="text-[11px] text-muted-foreground/70 truncate">{item.label}</span>
                                    <div className="flex items-baseline gap-0.5">
                                        <span className="text-lg font-semibold tabular-nums leading-tight">
                                            <AnimatedNumber value={item.value} />
                                        </span>
                                        {item.unit && (
                                            <span className="text-[11px] text-muted-foreground/60">{item.unit}</span>
                                        )}
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
