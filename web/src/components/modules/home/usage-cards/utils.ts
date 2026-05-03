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

    if (Math.abs(value) >= 1_000_000) {
        return `${(value / 1_000_000).toFixed(1)}M`;
    }

    if (Math.abs(value) >= 1_000) {
        return `${(value / 1_000).toFixed(1)}K`;
    }

    if (Number.isInteger(value)) {
        return value.toString();
    }

    return value.toFixed(1);
}

export function formatResetTime(resetAt: string): string {
    const date = dayjs(resetAt);
    const now = dayjs();

    if (date.isBefore(now)) {
        return date.format('MM-DD HH:mm');
    }

    const diffSec = date.diff(now, 'second');
    if (diffSec < 60) return `${diffSec}s`;
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h`;
    return date.fromNow();
}

export function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
    switch (status) {
        case 'ok': return 'secondary';
        case 'warning': return 'outline';
        case 'exhausted': return 'destructive';
        default: return 'outline';
    }
}
