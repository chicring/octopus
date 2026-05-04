'use client';

import { useCallback, useMemo, useState } from 'react';
import { useTranslations } from 'next-intl';
import { Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { useUsageCardList, useUsageCardTemplates, useDeleteUsageCard, useRefreshUsageCard, type UsageCard, type UsageMetric } from '@/api/endpoints/usage-card';
import { Card, CardContent, CardHeader, CardTitle, CardAction } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { toast } from '@/components/common/Toast';
import { UsageCardFormDialog } from './form';
import { formatMetricValue, formatResetTime, statusVariant } from './utils';

export function UsageCards() {
    const t = useTranslations('home.usageCard');
    const { data: cards, isLoading } = useUsageCardList();
    const { data: templates } = useUsageCardTemplates();
    const deleteCard = useDeleteUsageCard();
    const refreshCard = useRefreshUsageCard();

    const [editing, setEditing] = useState(false);
    const [formOpen, setFormOpen] = useState(false);
    const [editingCard, setEditingCard] = useState<UsageCard | null>(null);
    const [refreshingCardId, setRefreshingCardId] = useState<number | null>(null);

    // 从 API 模板数据构建 id→name 映射，避免硬编码
    const templateLabels = useMemo(() => {
        const map: Record<string, string> = {};
        if (templates) {
            for (const t of templates) {
                map[t.id] = t.name;
            }
        }
        return map;
    }, [templates]);

    const enabledCards = useMemo(() => cards?.filter(c => c.enabled) ?? [], [cards]);

    const handleDelete = useCallback((id: number) => {
        deleteCard.mutate(id, {
            onSuccess: () => toast.success(t('toast.deleteSuccess')),
            onError: () => toast.error(t('toast.deleteError')),
        });
    }, [deleteCard, t]);

    const handleRefresh = useCallback((id: number) => {
        setRefreshingCardId(id);
        refreshCard.mutate(id, {
            onSuccess: () => { toast.success(t('toast.refreshSuccess')); setRefreshingCardId(null); },
            onError: () => { toast.error(t('toast.refreshError')); setRefreshingCardId(null); },
            onSettled: () => setRefreshingCardId(null),
        });
    }, [refreshCard, t]);

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h2 className="text-lg font-bold">{t('title')}</h2>
                <div className="flex items-center gap-2">
                    {editing && (
                        <Button
                            variant="outline"
                            size="sm"
                            className="h-8 gap-1 rounded-xl"
                            onClick={() => { setEditingCard(null); setFormOpen(true); }}
                        >
                            <Plus className="size-3.5" />
                            {t('add')}
                        </Button>
                    )}
                    <Button
                        variant={editing ? 'default' : 'outline'}
                        size="sm"
                        className="h-8 gap-1 rounded-xl"
                        onClick={() => setEditing(!editing)}
                    >
                        <Pencil className="size-3.5" />
                        {t('edit')}
                    </Button>
                </div>
            </div>

            {isLoading ? (
                <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
                    <RefreshCw className="size-4 animate-spin mr-2" />
                    Loading...
                </div>
            ) : enabledCards.length === 0 && !editing ? (
                <div className="rounded-2xl border border-dashed border-border/60 py-8 text-center text-sm text-muted-foreground">
                    {t('noData')}
                </div>
            ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
                    {enabledCards.map(card => (
                        <UsageCardItem
                            key={card.id}
                            card={card}
                            editing={editing}
                            templateLabels={templateLabels}
                            refreshing={refreshingCardId === card.id}
                            onDelete={() => handleDelete(card.id)}
                            onRefresh={() => handleRefresh(card.id)}
                            onEdit={() => { setEditingCard(card); setFormOpen(true); }}
                        />
                    ))}
                    {editing && enabledCards.length === 0 && (
                        <button
                            onClick={() => { setEditingCard(null); setFormOpen(true); }}
                            className="flex min-h-[160px] items-center justify-center rounded-2xl border-2 border-dashed border-border/60 text-muted-foreground transition-colors hover:border-primary/40 hover:text-primary"
                        >
                            <Plus className="size-5 mr-2" />
                            {t('add')}
                        </button>
                    )}
                </div>
            )}

            <UsageCardFormDialog
                open={formOpen}
                onOpenChange={setFormOpen}
                card={editingCard}
            />
        </div>
    );
}

function UsageCardItem({
    card,
    editing,
    templateLabels,
    refreshing,
    onDelete,
    onRefresh,
    onEdit,
}: {
    card: UsageCard;
    editing: boolean;
    templateLabels: Record<string, string>;
    refreshing: boolean;
    onDelete: () => void;
    onRefresh: () => void;
    onEdit: () => void;
}) {
    const t = useTranslations('home.usageCard');
    const metrics = card.last_result?.metrics ?? [];
    const hasError = !!card.last_error;

    // 统一标题：template badge + 名称（去掉模板前缀）
    const badge = templateLabels[card.template_id];
    const displayTitle = badge ? card.name.replace(/^.+?\s*[-–—]\s*/, '') : card.name;

    return (
        <Card className="rounded-2xl gap-4 py-4">
            <CardHeader className="pb-0">
                <CardTitle className="text-base leading-tight">
                    {badge ? (
                        <span className="flex items-center gap-2">
                            <Badge variant="outline" className="text-[10px] px-1.5 py-0 font-medium">{badge}</Badge>
                            <span className="truncate text-sm text-muted-foreground">{displayTitle}</span>
                        </span>
                    ) : displayTitle}
                </CardTitle>
                <CardAction>
                    <div className="flex items-center gap-1.5">
                        {metrics.length > 0 && (
                            <Badge variant={statusVariant(metrics[0].status)} className="text-[10px] px-1.5 py-0">
                                {t(`status.${metrics[0].status}`)}
                            </Badge>
                        )}
                        {hasError && (
                            <Badge variant="destructive" className="text-[10px] px-1.5 py-0">
                                {t('status.error')}
                            </Badge>
                        )}
                        {!editing && (
                            <Button
                                variant="ghost"
                                size="icon"
                                className="size-7"
                                onClick={onRefresh}
                                disabled={refreshing}
                            >
                                <RefreshCw className={`size-3.5 ${refreshing ? 'animate-spin' : ''}`} />
                            </Button>
                        )}
                    </div>
                </CardAction>
            </CardHeader>

            <CardContent className="space-y-3 pt-0">
                {metrics.length === 0 && !hasError && (
                    <div className="text-xs text-muted-foreground text-center py-4">{t('metric.noData')}</div>
                )}

                {hasError && metrics.length === 0 && (
                    <div className="text-xs text-destructive text-center py-2">{card.last_error}</div>
                )}

                {metrics.slice(0, 6).map(metric => (
                    <MetricBar key={metric.id} metric={metric} />
                ))}

                {editing && (
                    <div className="flex items-center gap-2 pt-1 border-t border-border/40">
                        <Button variant="ghost" size="sm" className="h-7 text-xs gap-1 flex-1" onClick={onEdit}>
                            <Pencil className="size-3" />
                            {t('edit')}
                        </Button>
                        <Button variant="ghost" size="sm" className="h-7 text-xs gap-1 text-destructive hover:text-destructive" onClick={onDelete}>
                            <Trash2 className="size-3" />
                            {t('form.delete')}
                        </Button>
                    </div>
                )}
            </CardContent>
        </Card>
    );
}

function MetricBar({ metric }: { metric: UsageMetric }) {
    const t = useTranslations('home.usageCard');

    // plan 类型：标签行，只显示标签和值
    if (metric.unit === 'plan') {
        const planText = metric.message || metric.used?.toString() || 'Free';
        return (
            <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">{metric.label}</span>
                <span className="text-xs font-semibold text-foreground">{planText}</span>
            </div>
        );
    }

    // 统一进度条：percent 类型和其他类型共用
    const pct = metric.percent ?? (metric.limit && metric.used ? Math.min((metric.used / metric.limit) * 100, 100) : (metric.used ?? 0));
    const isExhausted = metric.status === 'exhausted';
    const isWarning = metric.status === 'warning';
    const barColor = isExhausted ? 'bg-destructive' : isWarning ? 'bg-amber-500' : 'bg-primary';

    const isPercent = metric.unit === 'percent';
    const usedText = isPercent ? `${pct.toFixed(1)}%` : formatMetricValue(metric.used, metric.unit);
    const remainingText = isPercent ? `${(100 - pct).toFixed(1)}%` : formatMetricValue(metric.remaining, metric.unit);
    const limitText = !isPercent && metric.limit != null ? formatMetricValue(metric.limit, metric.unit) : null;

    return (
        <div className="space-y-1.5">
            <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">{metric.label}</span>
                {metric.reset_at && (
                    <span className="text-[10px] text-muted-foreground">
                        {formatResetTime(metric.reset_at)}
                    </span>
                )}
            </div>
            <div className="h-2 rounded-full bg-muted/60 overflow-hidden">
                <div
                    className={`h-full rounded-full transition-all duration-500 ${barColor}`}
                    style={{ width: `${Math.min(pct, 100)}%` }}
                />
            </div>
            <div className="flex items-center justify-between text-[11px]">
                <span className="text-muted-foreground">
                    {t('metric.used')} <span className="text-foreground font-medium">{usedText}</span>
                </span>
                {limitText ? (
                    <span className="text-muted-foreground">
                        {t('metric.remaining')} <span className="text-foreground font-medium">{remainingText}</span>
                        <span className="ml-1 text-muted-foreground">/ {limitText}</span>
                    </span>
                ) : (
                    <span className="text-muted-foreground">
                        {t('metric.remaining')} <span className="text-foreground font-medium">{remainingText}</span>
                    </span>
                )}
            </div>
        </div>
    );
}
