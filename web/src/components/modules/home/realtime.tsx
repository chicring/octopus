'use client';

import { motion } from 'motion/react';
import { Gauge, Activity, Zap, Timer } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useStatsRealtime } from '@/api/endpoints/stats';
import { AnimatedNumber } from '@/components/common/AnimatedNumber';
import { EASING } from '@/lib/animations/fluid-transitions';

export function Realtime() {
    const { data } = useStatsRealtime();
    const t = useTranslations('home.realtime');

    const cards = [
        {
            label: t('rps'),
            description: t('rpsDescription'),
            value: data?.rps ?? 0,
            icon: Activity,
            bgColor: 'bg-chart-1/10',
            unit: 'req/s',
        },
        {
            label: t('rpm'),
            description: t('rpmDescription'),
            value: data?.rpm ?? 0,
            icon: Timer,
            bgColor: 'bg-chart-2/10',
            unit: 'req/min',
        },
        {
            label: t('tps'),
            description: t('tpsDescription'),
            value: data?.tps ?? 0,
            icon: Zap,
            bgColor: 'bg-chart-3/10',
            unit: 'tok/s',
        },
        {
            label: t('tpm'),
            description: t('tpmDescription'),
            value: data?.tpm ?? 0,
            icon: Gauge,
            bgColor: 'bg-chart-4/10',
            unit: 'tok/min',
        },
    ];

    return (
        <div className="grid grid-cols-2 xl:grid-cols-4 gap-4">
            {cards.map((card, index) => (
                <motion.section
                    key={card.label}
                    className="rounded-2xl bg-card border border-border/50 p-4 text-card-foreground flex flex-col gap-3"
                    initial={{ opacity: 0, y: 20, filter: 'blur(8px)' }}
                    animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
                    transition={{
                        duration: 0.5,
                        ease: EASING.easeOutExpo,
                        delay: index * 0.08,
                    }}
                >
                    <div className="flex items-center gap-2">
                        <div className={`flex items-center justify-center w-7 h-7 rounded-lg ${card.bgColor}`}>
                            <card.icon className="w-3.5 h-3.5 text-primary" />
                        </div>
                        <div className="flex flex-col min-w-0">
                            <h3 className="font-medium text-sm text-foreground truncate">{card.label}</h3>
                            <span className="text-xs text-muted-foreground truncate">{card.description}</span>
                        </div>
                    </div>

                    <div className="flex items-baseline gap-1">
                        <span className="text-2xl font-semibold tabular-nums">
                            <AnimatedNumber value={card.value.toString()} />
                        </span>
                        <span className="text-xs text-muted-foreground">{card.unit}</span>
                    </div>
                </motion.section>
            ))}
        </div>
    );
}
