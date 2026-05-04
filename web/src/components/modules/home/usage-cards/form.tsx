'use client';

import { useCallback, useEffect, useState } from 'react';
import { useTranslations } from 'next-intl';
import {
    useUsageCardTemplates,
    useCreateUsageCard,
    useUpdateUsageCard,
    useImportCodexChannelUsageCard,
    type UsageCard,
    type CreateUsageCardRequest,
    type UpdateUsageCardRequest,
} from '@/api/endpoints/usage-card';
import { useCodexChannels } from '@/api/endpoints/channel';
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { toast } from '@/components/common/Toast';
import { Plus, Trash2, ChevronDown, ChevronRight, Import } from 'lucide-react';

export function UsageCardFormDialog({
    open,
    onOpenChange,
    card,
}: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    card: UsageCard | null;
}) {
    const t = useTranslations('home.usageCard');
    const { data: templates } = useUsageCardTemplates();
    const createCard = useCreateUsageCard();
    const updateCard = useUpdateUsageCard();
    const importCodex = useImportCodexChannelUsageCard();
    const { data: codexChannels } = useCodexChannels();

    const isEditing = card !== null;

    const [name, setName] = useState('');
    const [templateId, setTemplateId] = useState('');
    const [account, setAccount] = useState('');
    const [endpoint, setEndpoint] = useState('');
    const [authType, setAuthType] = useState('none');
    const [authHeader, setAuthHeader] = useState('');
    const [secret, setSecret] = useState('');
    const [refreshInterval, setRefreshInterval] = useState(300);
    const [enabled, setEnabled] = useState(true);
    const [extraHeaders, setExtraHeaders] = useState<{ key: string; value: string }[]>([]);
    const [showImport, setShowImport] = useState(false);

    const selectedTemplate = templates?.find(t => t.id === templateId);

    useEffect(() => {
        if (open) {
            if (card) {
                setName(card.name);
                setTemplateId(card.template_id);
                setAccount(card.account);
                setEndpoint(card.endpoint);
                setAuthType(card.auth_type);
                setAuthHeader(card.auth_header);
                setSecret('');
                setRefreshInterval(card.refresh_interval_sec);
                setEnabled(card.enabled);
                setExtraHeaders(card.extra_headers ?? []);
            } else {
                setName('');
                setTemplateId('');
                setAccount('');
                setEndpoint('');
                setAuthType('none');
                setAuthHeader('');
                setSecret('');
                setRefreshInterval(300);
                setEnabled(true);
                setExtraHeaders([]);
            }
        }
    }, [open, card]);

    useEffect(() => {
        if (selectedTemplate && !isEditing) {
            setEndpoint(selectedTemplate.default_endpoint);
            setAuthType(selectedTemplate.auth_types.length > 0 ? selectedTemplate.auth_types[0] : 'none');
            setExtraHeaders(selectedTemplate.required_headers?.map(h => ({ key: h.key, value: h.value })) ?? []);
        }
    }, [selectedTemplate, isEditing]);

    const handleSubmit = useCallback(() => {
        if (!name.trim() || !templateId) return;

        if (isEditing) {
            const req: UpdateUsageCardRequest = {
                id: card.id,
                name: name.trim(),
                template_id: templateId,
                account: account.trim(),
                endpoint: endpoint.trim(),
                auth_type: authType,
                auth_header: authType === 'custom-header' ? authHeader.trim() : undefined,
                secret: secret || undefined,
                extra_headers: extraHeaders.filter(h => h.key.trim()),
                refresh_interval_sec: refreshInterval,
                enabled,
            };
            updateCard.mutate(req, {
                onSuccess: () => {
                    toast.success(t('toast.updateSuccess'));
                    onOpenChange(false);
                },
                onError: () => toast.error(t('toast.updateError')),
            });
        } else {
            const req: CreateUsageCardRequest = {
                name: name.trim(),
                template_id: templateId,
                account: account.trim(),
                endpoint: endpoint.trim(),
                auth_type: authType,
                auth_header: authType === 'custom-header' ? authHeader.trim() : undefined,
                secret: secret,
                extra_headers: extraHeaders.filter(h => h.key.trim()),
                refresh_interval_sec: refreshInterval,
                enabled,
            };
            createCard.mutate(req, {
                onSuccess: () => {
                    toast.success(t('toast.createSuccess'));
                    onOpenChange(false);
                },
                onError: () => toast.error(t('toast.createError')),
            });
        }
    }, [name, templateId, account, endpoint, authType, authHeader, secret, extraHeaders, refreshInterval, enabled, isEditing, card, createCard, updateCard, t, onOpenChange]);

    const isPending = createCard.isPending || updateCard.isPending;
    const showSecret = authType !== 'none';
    const showAuthHeader = authType === 'custom-header';

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md rounded-2xl">
                <DialogHeader>
                    <DialogTitle>{isEditing ? t('form.editTitle') : t('form.title')}</DialogTitle>
                </DialogHeader>

                <div className="space-y-4">
                    {/* Import from Codex Channel (create only) */}
                    {!isEditing && (
                        <div className="space-y-2">
                            <Button
                                variant="outline"
                                size="sm"
                                className="h-8 gap-1.5 rounded-xl w-full"
                                type="button"
                                onClick={() => setShowImport(!showImport)}
                            >
                                {showImport ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
                                <Import className="size-3.5" />
                                {t('importCodexChannel')}
                            </Button>
                            {showImport && (
                                <div className="rounded-xl border border-border/60 p-3 space-y-2 max-h-48 overflow-y-auto">
                                    {codexChannels && codexChannels.length > 0 ? (
                                        codexChannels.map(channel => (
                                            <div key={channel.id} className="space-y-1">
                                                <div className="text-xs font-medium text-foreground">{channel.name}</div>
                                                {channel.keys.map(key => (
                                                    <div key={key.id} className="flex items-center justify-between pl-3">
                                                        <span className="text-xs text-muted-foreground truncate">
                                                            {key.remark || `Key #${key.id}`}
                                                        </span>
                                                        <Button
                                                            variant="ghost"
                                                            size="sm"
                                                            className="h-6 text-xs px-2"
                                                            type="button"
                                                            disabled={importCodex.isPending}
                                                            onClick={() => {
                                                                importCodex.mutate(
                                                                    { channel_id: channel.id, key_id: key.id },
                                                                    {
                                                                        onSuccess: () => {
                                                                            toast.success(t('importSuccess'));
                                                                            onOpenChange(false);
                                                                        },
                                                                        onError: () => toast.error(t('importError')),
                                                                    },
                                                                );
                                                            }}
                                                        >
                                                            {t('form.create')}
                                                        </Button>
                                                    </div>
                                                ))}
                                            </div>
                                        ))
                                    ) : (
                                        <div className="text-xs text-muted-foreground text-center py-2">
                                            {t('noCodexChannels')}
                                        </div>
                                    )}
                                </div>
                            )}
                        </div>
                    )}

                    {/* Template */}
                    <div className="space-y-1.5">
                        <Label className="text-xs">{t('form.template')}</Label>
                        <Select value={templateId} onValueChange={setTemplateId}>
                            <SelectTrigger className="rounded-xl">
                                <SelectValue placeholder={t('form.selectTemplate')} />
                            </SelectTrigger>
                            <SelectContent className="rounded-xl">
                                {templates?.map(tpl => (
                                    <SelectItem key={tpl.id} value={tpl.id} className="rounded-xl">
                                        {tpl.name}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                        {selectedTemplate && (
                            <p className="text-[11px] text-muted-foreground">{selectedTemplate.description}</p>
                        )}
                        {templateId === 'codex-usage' && !isEditing && (
                            <p className="text-[11px] text-amber-600 dark:text-amber-400">{t('codexSecretHint')}</p>
                        )}
                    </div>

                    {/* Name */}
                    <div className="space-y-1.5">
                        <Label className="text-xs">{t('form.name')}</Label>
                        <Input
                            value={name}
                            onChange={e => setName(e.target.value)}
                            placeholder={t('form.namePlaceholder')}
                            className="rounded-xl"
                        />
                    </div>

                    {/* Account */}
                    <div className="space-y-1.5">
                        <Label className="text-xs">{t('form.account')}</Label>
                        <Input
                            value={account}
                            onChange={e => setAccount(e.target.value)}
                            placeholder={t('form.accountPlaceholder')}
                            className="rounded-xl"
                        />
                    </div>

                    {/* Endpoint */}
                    <div className="space-y-1.5">
                        <Label className="text-xs">{t('form.endpoint')}</Label>
                        <Input
                            value={endpoint}
                            onChange={e => setEndpoint(e.target.value)}
                            placeholder={t('form.endpointPlaceholder')}
                            className="rounded-xl"
                        />
                    </div>

                    {/* Auth Type */}
                    <div className="space-y-1.5">
                        <Label className="text-xs">{t('form.authType')}</Label>
                        <Select value={authType} onValueChange={setAuthType}>
                            <SelectTrigger className="rounded-xl">
                                <SelectValue placeholder={t('form.selectAuth')} />
                            </SelectTrigger>
                            <SelectContent className="rounded-xl">
                                <SelectItem value="none" className="rounded-xl">{t('form.authNone')}</SelectItem>
                                <SelectItem value="bearer" className="rounded-xl">{t('form.authBearer')}</SelectItem>
                                <SelectItem value="x-api-key" className="rounded-xl">{t('form.authApiKey')}</SelectItem>
                                <SelectItem value="custom-header" className="rounded-xl">{t('form.authCustomHeader')}</SelectItem>
                                <SelectItem value="cookie" className="rounded-xl">{t('form.authCookie')}</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>

                    {/* Auth Header (custom-header only) */}
                    {showAuthHeader && (
                        <div className="space-y-1.5">
                            <Label className="text-xs">{t('form.authHeaderName')}</Label>
                            <Input
                                value={authHeader}
                                onChange={e => setAuthHeader(e.target.value)}
                                placeholder={t('form.authHeaderPlaceholder')}
                                className="rounded-xl"
                            />
                        </div>
                    )}

                    {/* Secret */}
                    {showSecret && (
                        <div className="space-y-1.5">
                            <Label className="text-xs">{t('form.secret')}</Label>
                            {authType === 'cookie' ? (
                                <textarea
                                    value={secret}
                                    onChange={e => setSecret(e.target.value)}
                                    placeholder={isEditing ? t('form.secretHint') : t('form.secretPlaceholder')}
                                    className="flex min-h-[80px] w-full rounded-xl border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none transition-[color,box-shadow] placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] resize-y"
                                />
                            ) : (
                                <Input
                                    type="password"
                                    value={secret}
                                    onChange={e => setSecret(e.target.value)}
                                    placeholder={isEditing ? t('form.secretHint') : t('form.secretPlaceholder')}
                                    className="rounded-xl"
                                />
                            )}
                        </div>
                    )}

                    {/* Refresh Interval */}
                    <div className="space-y-1.5">
                        <Label className="text-xs">{t('form.refreshInterval')}</Label>
                        <Input
                            type="number"
                            value={refreshInterval}
                            onChange={e => setRefreshInterval(Number(e.target.value) || 300)}
                            className="rounded-xl"
                        />
                    </div>

                    {/* Extra Headers */}
                    <div className="space-y-2">
                        <div className="flex items-center justify-between">
                            <Label className="text-xs">{t('form.extraHeaders')}</Label>
                            <Button
                                variant="ghost"
                                size="sm"
                                className="h-6 text-xs gap-1"
                                type="button"
                                onClick={() => setExtraHeaders([...extraHeaders, { key: '', value: '' }])}
                            >
                                <Plus className="size-3" />
                                {t('form.addHeader')}
                            </Button>
                        </div>
                        {extraHeaders.map((h, i) => (
                            <div key={i} className="flex items-center gap-2">
                                <Input
                                    value={h.key}
                                    onChange={e => {
                                        const next = [...extraHeaders];
                                        next[i] = { ...next[i], key: e.target.value };
                                        setExtraHeaders(next);
                                    }}
                                    placeholder={t('form.headerKeyPlaceholder')}
                                    className="rounded-xl flex-1"
                                />
                                <Input
                                    value={h.value}
                                    onChange={e => {
                                        const next = [...extraHeaders];
                                        next[i] = { ...next[i], value: e.target.value };
                                        setExtraHeaders(next);
                                    }}
                                    placeholder={t('form.headerValuePlaceholder')}
                                    className="rounded-xl flex-1"
                                />
                                <Button
                                    variant="ghost"
                                    size="icon"
                                    className="size-7 shrink-0 text-muted-foreground hover:text-destructive"
                                    type="button"
                                    onClick={() => setExtraHeaders(extraHeaders.filter((_, j) => j !== i))}
                                >
                                    <Trash2 className="size-3" />
                                </Button>
                            </div>
                        ))}
                    </div>

                    {/* Enabled */}
                    <div className="flex items-center gap-2">
                        <Switch checked={enabled} onCheckedChange={setEnabled} />
                        <Label className="text-xs">{t('form.enabled')}</Label>
                    </div>
                </div>

                <DialogFooter>
                    <Button variant="outline" className="rounded-xl" onClick={() => onOpenChange(false)} disabled={isPending}>
                        {t('form.cancel')}
                    </Button>
                    <Button className="rounded-xl" onClick={handleSubmit} disabled={isPending || !name.trim() || !templateId}>
                        {isPending ? '...' : isEditing ? t('form.save') : t('form.create')}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
