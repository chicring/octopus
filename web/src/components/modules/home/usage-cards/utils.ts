import { type UsageMetric } from '@/api/endpoints/usage-card';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';

dayjs.extend(relativeTime);

export function formatMetricValue(value: number | null | undefined, unit: string): string {
    if (value == null) return '-';

    if (unit === 'usd') {
        return `$${value.toFixed(2)}`;
    }

    if (unit === 'percent') {
        return `${value.toFixed(1)}%`;
    }

    if (Number.isInteger(value)) {
        return value.toLocaleString();
    }

    return value.toLocaleString(undefined, { maximumFractionDigits: 2 });
}

export function formatResetTime(resetAt: string): string {
    const date = dayjs(resetAt);
    if (!date.isValid()) return '-';

    // 始终显示具体时间，如 "05-10 20:26"
    return date.format('MM-DD HH:mm');
}

export function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
    switch (status) {
        case 'ok': return 'secondary';
        case 'warning': return 'outline';
        case 'exhausted': return 'destructive';
        default: return 'outline';
    }
}
