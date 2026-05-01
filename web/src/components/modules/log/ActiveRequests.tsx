'use client';

import { useMemo, useState, useEffect, useCallback } from 'react';
import { Activity, Clock, KeyRound, ArrowRight } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { getModelIcon } from '@/lib/model-icons';
import type { ActiveRequest, ActiveRequestStatus } from '@/api/endpoints/log';

function formatElapsed(startTime: number, now: number): string {
    const elapsed = Math.floor(now / 1000 - startTime);
    if (elapsed < 0) return '0s';
    if (elapsed < 60) return `${elapsed}s`;
    const min = Math.floor(elapsed / 60);
    const sec = elapsed % 60;
    return `${min}m${sec}s`;
}

const STATUS_CONFIG: Record<ActiveRequestStatus, { labelKey: string; color: string; dotClass: string }> = {
    forwarding: { labelKey: 'forwarding', color: 'text-blue-500', dotClass: 'bg-blue-500' },
    waiting_first_token: { labelKey: 'waitingFirstToken', color: 'text-amber-500', dotClass: 'bg-amber-500' },
    streaming: { labelKey: 'streaming', color: 'text-green-500', dotClass: 'bg-green-500' },
    processing: { labelKey: 'processing', color: 'text-purple-500', dotClass: 'bg-purple-500' },
};

function ActiveRequestItem({ request, now }: { request: ActiveRequest; now: number }) {
    const t = useTranslations('log.activeRequest');
    const { Avatar: ModelAvatar } = useMemo(
        () => getModelIcon(request.request_model),
        [request.request_model]
    );
    const statusConfig = STATUS_CONFIG[request.status] ?? STATUS_CONFIG.forwarding;
    const elapsed = formatElapsed(request.start_time, now);

    return (
        <div className="flex items-center gap-2.5 px-2.5 py-2 rounded-lg hover:bg-muted/50 transition-colors">
            <ModelAvatar size={28} />
            <div className="flex-1 min-w-0 flex flex-col gap-0.5">
                <div className="flex items-center gap-1.5 text-xs">
                    <span className="font-semibold text-card-foreground truncate">{request.request_model}</span>
                    {request.channel_name && (
                        <>
                            <ArrowRight className="size-3 shrink-0 text-muted-foreground/50" />
                            <span className="truncate text-muted-foreground">{request.channel_name}</span>
                        </>
                    )}
                </div>
                <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
                    <span className={cn('flex items-center gap-1', statusConfig.color)}>
                        <span className={cn('size-1.5 rounded-full animate-pulse', statusConfig.dotClass)} />
                        {t(statusConfig.labelKey)}
                    </span>
                    <span className="flex items-center gap-0.5 tabular-nums">
                        <Clock className="size-3" />
                        {elapsed}
                    </span>
                    {request.api_key_name && (
                        <span className="flex items-center gap-0.5 truncate">
                            <KeyRound className="size-3 shrink-0" />
                            <span className="truncate">{request.api_key_name}</span>
                        </span>
                    )}
                    {request.attempt_count > 1 && (
                        <Badge variant="secondary" className="h-4 px-1 text-[10px]">
                            x{request.attempt_count}
                        </Badge>
                    )}
                </div>
            </div>
        </div>
    );
}

interface ActiveRequestsPopoverProps {
    activeRequests: ActiveRequest[];
}

export function ActiveRequestsPopover({ activeRequests }: ActiveRequestsPopoverProps) {
    const t = useTranslations('log.activeRequest');
    const count = activeRequests.length;

    // 单一计时器驱动所有子项的耗时显示
    const [now, setNow] = useState(() => Date.now());
    useEffect(() => {
        if (count === 0) return;
        const timer = setInterval(() => setNow(Date.now()), 1000);
        return () => clearInterval(timer);
    }, [count > 0]); // eslint-disable-line react-hooks/exhaustive-deps

    return (
        <Popover>
            <PopoverTrigger asChild>
                <Button
                    variant="outline"
                    size="sm"
                    className={cn(
                        'relative gap-2 px-3 h-9 rounded-xl border-border shrink-0',
                        count > 0 && 'border-primary/30 bg-primary/5'
                    )}
                >
                    <Activity className={cn('h-4 w-4', count > 0 ? 'text-primary animate-pulse' : 'text-muted-foreground')} />
                    <span className={cn('text-sm sr-only sm:not-sr-only', count > 0 ? 'text-foreground' : 'text-muted-foreground')}>
                        {t('active')}
                    </span>
                    {count > 0 && (
                        <Badge className="absolute -top-1.5 -right-1.5 h-4 min-w-4 px-1 text-[10px] font-bold">
                            {count}
                        </Badge>
                    )}
                </Button>
            </PopoverTrigger>
            <PopoverContent
                className="w-80 p-0 rounded-xl"
                align="start"
            >
                <div className="px-3 py-2 border-b border-border">
                    <span className="text-sm font-medium text-card-foreground">
                        {t('title', { count })}
                    </span>
                </div>
                <div className="max-h-72 overflow-y-auto py-1">
                    {count === 0 ? (
                        <div className="py-6 text-center text-sm text-muted-foreground">
                            {t('empty')}
                        </div>
                    ) : (
                        activeRequests.map((req) => (
                            <ActiveRequestItem key={req.id} request={req} now={now} />
                        ))
                    )}
                </div>
            </PopoverContent>
        </Popover>
    );
}
