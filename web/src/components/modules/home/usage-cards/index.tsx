'use client';

import { useCallback, useMemo, useRef, useState } from 'react';
import { useTranslations } from 'next-intl';
import { Pencil, Plus, RefreshCw, Trash2, Import, X } from 'lucide-react';
import { useUsageCardList, useUsageCardTemplates, useDeleteUsageCard, useRefreshUsageCard, useBatchDeleteUsageCard, useBatchImportCodexChannel, type UsageCard, type UsageMetric } from '@/api/endpoints/usage-card';
import { useCodexChannels } from '@/api/endpoints/channel';
import { Card, CardContent, CardHeader, CardTitle, CardAction } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from '@/components/ui/dialog';
import { toast } from '@/components/common/Toast';
import { UsageCardFormDialog } from './form';
import { formatMetricValue, formatResetTime, statusVariant } from './utils';

const BATCH_SIZE = 5;
const BATCH_DELAY_MS = 500;

export function UsageCards() {
    const t = useTranslations('home.usageCard');
    const { data: cards, isLoading } = useUsageCardList();
    const { data: templates } = useUsageCardTemplates();
    const deleteCard = useDeleteUsageCard();
    const refreshCard = useRefreshUsageCard();
    const batchDeleteMutation = useBatchDeleteUsageCard();
    const batchImportMutation = useBatchImportCodexChannel();
    const { data: codexChannels } = useCodexChannels();

    const [editing, setEditing] = useState(false);
    const [formOpen, setFormOpen] = useState(false);
    const [editingCard, setEditingCard] = useState<UsageCard | null>(null);
    const [refreshingCardId, setRefreshingCardId] = useState<number | null>(null);
    const [batchRefreshing, setBatchRefreshing] = useState(false);
    const [batchRefreshProgress, setBatchRefreshProgress] = useState({ current: 0, total: 0 });
    const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
    const [batchImportOpen, setBatchImportOpen] = useState(false);
    const [selectedDeleteIds, setSelectedDeleteIds] = useState<Set<number>>(new Set());
    const [selectedImportItems, setSelectedImportItems] = useState<Set<string>>(new Set());
    const batchRefreshAbortRef = useRef(false);

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

    const handleBatchRefresh = useCallback(async () => {
        if (enabledCards.length === 0) return;
        setBatchRefreshing(true);
        batchRefreshAbortRef.current = false;
        const total = enabledCards.length;
        setBatchRefreshProgress({ current: 0, total });

        for (let i = 0; i < total; i += BATCH_SIZE) {
            if (batchRefreshAbortRef.current) break;
            const batch = enabledCards.slice(i, i + BATCH_SIZE);
            await Promise.allSettled(
                batch.map(card => refreshCard.mutateAsync(card.id).catch(() => {}))
            );
            setBatchRefreshProgress({ current: Math.min(i + BATCH_SIZE, total), total });
            if (i + BATCH_SIZE < total && !batchRefreshAbortRef.current) {
                await new Promise(r => setTimeout(r, BATCH_DELAY_MS));
            }
        }
        setBatchRefreshing(false);
        if (!batchRefreshAbortRef.current) {
            toast.success(t('toast.batchRefreshSuccess'));
        }
    }, [enabledCards, refreshCard, t]);

    const handleAbortBatchRefresh = useCallback(() => {
        batchRefreshAbortRef.current = true;
        setBatchRefreshing(false);
    }, []);

    const handleBatchDelete = useCallback(() => {
        if (selectedDeleteIds.size === 0) return;
        batchDeleteMutation.mutate(Array.from(selectedDeleteIds), {
            onSuccess: () => {
                toast.success(t('toast.batchDeleteSuccess', { count: selectedDeleteIds.size }));
                setSelectedDeleteIds(new Set());
                setBatchDeleteOpen(false);
            },
            onError: () => toast.error(t('toast.batchDeleteError')),
        });
    }, [batchDeleteMutation, selectedDeleteIds, t]);

    const handleBatchImport = useCallback(() => {
        if (selectedImportItems.size === 0) return;
        const items = Array.from(selectedImportItems).map(key => {
            const [channelId, keyId] = key.split(':').map(Number);
            return { channel_id: channelId, key_id: keyId };
        });
        batchImportMutation.mutate(items, {
            onSuccess: () => {
                toast.success(t('toast.batchImportSuccess', { count: items.length }));
                setSelectedImportItems(new Set());
                setBatchImportOpen(false);
            },
            onError: () => toast.error(t('toast.batchImportError')),
        });
    }, [batchImportMutation, selectedImportItems, t]);

    const toggleDeleteId = useCallback((id: number) => {
        setSelectedDeleteIds(prev => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            return next;
        });
    }, []);

    const toggleImportItem = useCallback((key: string) => {
        setSelectedImportItems(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    }, []);

    const toggleAllDelete = useCallback(() => {
        if (selectedDeleteIds.size === enabledCards.length) {
            setSelectedDeleteIds(new Set());
        } else {
            setSelectedDeleteIds(new Set(enabledCards.map(c => c.id)));
        }
    }, [selectedDeleteIds, enabledCards]);

    const toggleAllImport = useCallback(() => {
        const allKeys: string[] = [];
        if (codexChannels) {
            for (const ch of codexChannels) {
                for (const key of ch.keys) {
                    allKeys.push(`${ch.id}:${key.id}`);
                }
            }
        }
        if (selectedImportItems.size === allKeys.length && allKeys.length > 0) {
            setSelectedImportItems(new Set());
        } else {
            setSelectedImportItems(new Set(allKeys));
        }
    }, [selectedImportItems, codexChannels]);

    const openBatchDeleteDialog = useCallback(() => {
        setSelectedDeleteIds(new Set());
        setBatchDeleteOpen(true);
    }, []);

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h2 className="text-lg font-bold">{t('title')}</h2>
                <div className="flex items-center gap-2">
                    {!batchRefreshing && (
                        <Button
                            variant="outline"
                            size="sm"
                            className="h-8 gap-1 rounded-xl"
                            onClick={handleBatchRefresh}
                            disabled={enabledCards.length === 0}
                        >
                            <RefreshCw className="size-3.5" />
                            {t('batchRefresh')}
                        </Button>
                    )}
                    {batchRefreshing && (
                        <div className="flex items-center gap-2">
                            <span className="text-xs text-muted-foreground">
                                {t('batchRefreshProgress', { current: batchRefreshProgress.current, total: batchRefreshProgress.total })}
                            </span>
                            <Button
                                variant="outline"
                                size="sm"
                                className="h-8 gap-1 rounded-xl text-destructive"
                                onClick={handleAbortBatchRefresh}
                            >
                                <X className="size-3.5" />
                                {t('cancel')}
                            </Button>
                        </div>
                    )}
                    {editing ? (
                        <Button
                            variant="default"
                            size="sm"
                            className="h-8 gap-1 rounded-xl"
                            onClick={() => setEditing(false)}
                        >
                            <X className="size-3.5" />
                            {t('exitEdit')}
                        </Button>
                    ) : (
                        <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                                <Button variant="outline" size="sm" className="h-8 gap-1 rounded-xl">
                                    <Pencil className="size-3.5" />
                                    {t('edit')}
                                </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="rounded-xl">
                                <DropdownMenuItem onClick={() => { setEditingCard(null); setFormOpen(true); }}>
                                    <Plus className="size-3.5" />
                                    {t('add')}
                                </DropdownMenuItem>
                                <DropdownMenuItem onClick={() => setBatchImportOpen(true)}>
                                    <Import className="size-3.5" />
                                    {t('batchImport')}
                                </DropdownMenuItem>
                                <DropdownMenuItem onClick={openBatchDeleteDialog}>
                                    <Trash2 className="size-3.5" />
                                    {t('batchDelete')}
                                </DropdownMenuItem>
                                <DropdownMenuSeparator />
                                <DropdownMenuItem onClick={() => setEditing(true)}>
                                    <Pencil className="size-3.5" />
                                    {t('enterEdit')}
                                </DropdownMenuItem>
                            </DropdownMenuContent>
                        </DropdownMenu>
                    )}
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

            {/* 批量删除弹窗 */}
            <Dialog open={batchDeleteOpen} onOpenChange={setBatchDeleteOpen}>
                <DialogContent className="max-w-md rounded-2xl">
                    <DialogHeader>
                        <DialogTitle>{t('batchDeleteTitle')}</DialogTitle>
                    </DialogHeader>
                    <div className="flex items-center gap-2 mb-2">
                        <Checkbox
                            checked={selectedDeleteIds.size === enabledCards.length && enabledCards.length > 0}
                            onCheckedChange={toggleAllDelete}
                        />
                        <span className="text-xs text-muted-foreground">
                            {t('selectedCount', { count: selectedDeleteIds.size })}
                        </span>
                    </div>
                    <div className="space-y-1 max-h-64 overflow-y-auto">
                        {enabledCards.map(card => (
                            <label key={card.id} className="flex items-center gap-2 cursor-pointer rounded-lg p-1.5 hover:bg-muted/50">
                                <Checkbox
                                    checked={selectedDeleteIds.has(card.id)}
                                    onCheckedChange={() => toggleDeleteId(card.id)}
                                />
                                <span className="text-sm truncate">{card.name}</span>
                            </label>
                        ))}
                    </div>
                    <DialogFooter>
                        <Button variant="outline" className="rounded-xl" onClick={() => setBatchDeleteOpen(false)}>
                            {t('cancel')}
                        </Button>
                        <Button
                            variant="destructive"
                            className="rounded-xl"
                            disabled={selectedDeleteIds.size === 0}
                            onClick={handleBatchDelete}
                        >
                            {t('deleteSelected')} ({selectedDeleteIds.size})
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            {/* 批量导入 Codex 弹窗 */}
            <Dialog open={batchImportOpen} onOpenChange={setBatchImportOpen}>
                <DialogContent className="max-w-md rounded-2xl">
                    <DialogHeader>
                        <DialogTitle>{t('batchImportTitle')}</DialogTitle>
                    </DialogHeader>
                    {codexChannels && codexChannels.length > 0 && (
                        <div className="flex items-center gap-2 mb-2">
                            <Checkbox
                                checked={(() => {
                                    const allKeys: string[] = [];
                                    for (const ch of codexChannels) {
                                        for (const key of ch.keys) {
                                            allKeys.push(`${ch.id}:${key.id}`);
                                        }
                                    }
                                    return allKeys.length > 0 && selectedImportItems.size === allKeys.length;
                                })()}
                                onCheckedChange={toggleAllImport}
                            />
                            <span className="text-xs text-muted-foreground">
                                {t('selectedCount', { count: selectedImportItems.size })}
                            </span>
                        </div>
                    )}
                    <div className="space-y-3 max-h-64 overflow-y-auto">
                        {codexChannels && codexChannels.length > 0 ? (
                            codexChannels.map(channel => (
                                <div key={channel.id} className="space-y-1">
                                    <div className="text-xs font-medium text-foreground">{channel.name}</div>
                                    {channel.keys.map(key => {
                                        const itemKey = `${channel.id}:${key.id}`;
                                        return (
                                            <label key={key.id} className="flex items-center gap-2 cursor-pointer rounded-lg pl-3 p-1.5 hover:bg-muted/50">
                                                <Checkbox
                                                    checked={selectedImportItems.has(itemKey)}
                                                    onCheckedChange={() => toggleImportItem(itemKey)}
                                                />
                                                <span className="text-xs text-muted-foreground truncate">
                                                    {key.remark || `Key #${key.id}`}
                                                </span>
                                            </label>
                                        );
                                    })}
                                </div>
                            ))
                        ) : (
                            <div className="text-xs text-muted-foreground text-center py-4">
                                {t('noCodexChannels')}
                            </div>
                        )}
                    </div>
                    <DialogFooter>
                        <Button variant="outline" className="rounded-xl" onClick={() => setBatchImportOpen(false)}>
                            {t('cancel')}
                        </Button>
                        <Button
                            className="rounded-xl"
                            disabled={selectedImportItems.size === 0}
                            onClick={handleBatchImport}
                        >
                            {t('importSelected')} ({selectedImportItems.size})
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
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

    if (metric.unit === 'plan') {
        const planText = metric.message || metric.used?.toString() || 'Free';
        return (
            <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">{metric.label}</span>
                <span className="text-xs font-semibold text-foreground">{planText}</span>
            </div>
        );
    }

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
