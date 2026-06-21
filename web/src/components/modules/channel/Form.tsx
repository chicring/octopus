import { AutoGroupType, ChannelType, type Channel, useFetchModel, useTestChannelModelsByConfig, type TestModelResult } from '@/api/endpoints/channel';
import { useProviderList, type ProviderInfo, type AuthResult } from '@/api/endpoints/provider';
import { OAuthPanel } from './OAuthPanel';
import { AuthFileImportPanel } from './AuthFileImportPanel';
import { parseOAuthLabel } from './utils';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from '@/components/ui/dialog';
import { Checkbox } from '@/components/ui/checkbox';
import { toast } from '@/components/common/Toast';
import { useTranslations } from 'next-intl';
import { useEffect, useMemo, useRef, useState } from 'react';
import { RefreshCw, X, Plus, CheckCircle2, XCircle, Loader2, AlertTriangle } from 'lucide-react';

export interface ChannelKeyFormItem {
    id?: number;
    enabled: boolean;
    channel_key: string;
    status_code?: number;
    last_use_time_stamp?: number;
    total_cost?: number;
    total_requests?: number;
    total_input_token?: number;
    total_output_token?: number;
    remark?: string;
}

export interface ChannelFormData {
    name: string;
    base_urls: Channel['base_urls'];
    custom_header: Channel['custom_header'];
    channel_proxy: string;
    param_override: string;
    keys: ChannelKeyFormItem[];
    model: string;
    custom_model: string;
    enabled: boolean;
    proxy: boolean;
    auto_sync: boolean;
    auto_group: AutoGroupType;
    match_regex: string;
}

export interface ChannelFormProps {
    formData: ChannelFormData;
    onFormDataChange: (data: ChannelFormData) => void;
    onSubmit: (event: React.FormEvent<HTMLFormElement>) => void;
    isPending: boolean;
    submitText: string;
    pendingText: string;
    onCancel?: () => void;
    cancelText?: string;
    idPrefix?: string;
    channelId?: number;
    hideSubmit?: boolean;
}

import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "@/components/ui/accordion";

export function ChannelForm({
    formData,
    onFormDataChange,
    onSubmit,
    isPending,
    submitText,
    pendingText,
    onCancel,
    cancelText,
    idPrefix = 'channel',
    channelId,
    hideSubmit,
}: ChannelFormProps) {
    const t = useTranslations('channel.form');
    const { data: providers } = useProviderList();

    // Derive the currently selected provider info from the first BaseUrl
    const firstBaseUrl = formData.base_urls?.[0];
    const selectedProvider = useMemo<ProviderInfo | undefined>(
        () => providers?.find((p) => p.id === firstBaseUrl?.provider_id),
        [providers, firstBaseUrl?.provider_id],
    );
    const isOAuthProvider = selectedProvider && selectedProvider.auth_type !== 'manual';

    // Ensure the form always shows at least 1 row for base_urls / keys / custom_header.
    // This avoids "empty list" UI and also keeps URL + APIKEY layout consistent.
    useEffect(() => {
        if (!formData.base_urls || formData.base_urls.length === 0) {
            onFormDataChange({ ...formData, base_urls: [{ url: '', delay: 0, type: ChannelType.OpenAIChat }] });
            return;
        }
        if (!formData.keys || formData.keys.length === 0) {
            onFormDataChange({ ...formData, keys: [{ enabled: true, channel_key: '' }] });
            return;
        }
        if (!formData.custom_header || formData.custom_header.length === 0) {
            onFormDataChange({ ...formData, custom_header: [{ header_key: '', header_value: '' }] });
        }
    }, [formData, onFormDataChange]);

    const autoModels = useMemo(() => formData.model
        ? formData.model.split(',').map((m) => m.trim()).filter(Boolean)
        : [], [formData.model]);
    const customModels = useMemo(() => formData.custom_model
        ? formData.custom_model.split(',').map((m) => m.trim()).filter(Boolean)
        : [], [formData.custom_model]);
    const [inputValue, setInputValue] = useState('');
    const inputRef = useRef<HTMLInputElement>(null);

    // Model selection dialog state
    const [showModelDialog, setShowModelDialog] = useState(false);
    const [fetchedModels, setFetchedModels] = useState<string[]>([]);
    const [dialogSelectedModels, setDialogSelectedModels] = useState<Set<string>>(new Set());
    const [dialogSearch, setDialogSearch] = useState('');

    // Model test state
    const testModels = useTestChannelModelsByConfig();
    const [testResults, setTestResults] = useState<TestModelResult[]>([]);

    const fetchModel = useFetchModel();

    const effectiveKey =
        formData.keys.find((k) => k.enabled && k.channel_key.trim())?.channel_key.trim() || '';

    const updateModels = (nextAuto: string[], nextCustom: string[]) => {
        const model = nextAuto.join(',');
        const custom_model = nextCustom.join(',');
        if (formData.model === model && formData.custom_model === custom_model) return;
        onFormDataChange({ ...formData, model, custom_model });
    };

    const handleRefreshModels = async () => {
        if (!formData.base_urls?.[0]?.url || !effectiveKey) return;
        fetchModel.mutate(
            {
                base_urls: formData.base_urls,
                keys: formData.keys
                    .filter((k) => k.channel_key.trim())
                    .map((k) => ({ enabled: k.enabled, channel_key: k.channel_key.trim() })),
                proxy: formData.proxy,
                channel_proxy: formData.channel_proxy?.trim() || null,
                match_regex: formData.match_regex.trim() || null,
                custom_header: formData.custom_header?.filter((h) => h.header_key.trim()) || [],
            },
            {
                onSuccess: (data) => {
                    if (data && data.length > 0) {
                        setFetchedModels(data);
                        setDialogSelectedModels(new Set(autoModels));
                        setDialogSearch('');
                        setShowModelDialog(true);
                    } else {
                        toast.warning(t('modelRefreshEmpty'));
                    }
                },
                onError: (error) => {
                    const errorMessage = error instanceof Error ? error.message : String(error);
                    toast.error(t('modelRefreshFailed'), { description: errorMessage });
                },
            }
        );
    };

    const handleDialogConfirm = () => {
        updateModels(Array.from(dialogSelectedModels), customModels);
        setShowModelDialog(false);
        toast.success(t('modelRefreshSuccess'));
    };

    const filteredDialogModels = useMemo(() => {
        const q = dialogSearch.toLowerCase();
        return q ? fetchedModels.filter((m) => m.toLowerCase().includes(q)) : fetchedModels;
    }, [fetchedModels, dialogSearch]);

    const dialogAllSelected = filteredDialogModels.length > 0 && filteredDialogModels.every((m) => dialogSelectedModels.has(m));

    const handleDialogToggleAll = () => {
        setDialogSelectedModels((prev) => {
            const next = new Set(prev);
            if (dialogAllSelected) {
                filteredDialogModels.forEach((m) => next.delete(m));
            } else {
                filteredDialogModels.forEach((m) => next.add(m));
            }
            return next;
        });
    };

    const handleDialogToggleModel = (model: string) => {
        setDialogSelectedModels((prev) => {
            const next = new Set(prev);
            if (next.has(model)) {
                next.delete(model);
            } else {
                next.add(model);
            }
            return next;
        });
    };

    // Model test handlers
    const allModels = useMemo(() => [...autoModels, ...customModels], [autoModels, customModels]);

    const getTestConfig = () => ({
        base_urls: formData.base_urls,
        keys: formData.keys
            .filter((k) => k.channel_key.trim())
            .map((k) => ({ enabled: k.enabled, channel_key: k.channel_key.trim() })),
        proxy: formData.proxy,
        channel_proxy: formData.channel_proxy?.trim() || null,
        match_regex: formData.match_regex.trim() || null,
        custom_header: formData.custom_header?.filter((h) => h.header_key.trim()) || [],
    });

    const handleTestFirst = () => {
        if (allModels.length === 0) return;
        setTestResults([]);
        testModels.mutate({ ...getTestConfig(), models: [allModels[0]] }, {
            onSuccess: (data) => setTestResults(data),
            onError: (error) => {
                setTestResults([{ model: allModels[0], passed: false, error: error.message }]);
            },
        });
    };

    const handleTestAll = () => {
        if (allModels.length === 0) return;
        setTestResults([]);
        testModels.mutate({ ...getTestConfig(), models: allModels }, {
            onSuccess: (data) => setTestResults(data),
            onError: (error) => {
                setTestResults(allModels.map((m) => ({ model: m, passed: false, error: error.message })));
            },
        });
    };

    const handleAddModel = (model: string) => {
        const trimmedModel = model.trim();
        if (trimmedModel && !customModels.includes(trimmedModel) && !autoModels.includes(trimmedModel)) {
            updateModels(autoModels, [...customModels, trimmedModel]);
        }
        setInputValue('');
    };

    const handleRemoveAutoModel = (model: string) => {
        updateModels(autoModels.filter(m => m !== model), customModels);
    };

    const handleRemoveCustomModel = (model: string) => {
        updateModels(autoModels, customModels.filter(m => m !== model));
    };

    const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            if (inputValue.trim()) handleAddModel(inputValue);
        }
    };

    const handleAddKey = () => {
        onFormDataChange({
            ...formData,
            keys: [...formData.keys, { enabled: true, channel_key: '' }],
        });
    };

    const handleUpdateKey = (idx: number, patch: Partial<ChannelKeyFormItem>) => {
        const next = formData.keys.map((k, i) => (i === idx ? { ...k, ...patch } : k));
        onFormDataChange({ ...formData, keys: next });
    };

    const handleRemoveKey = (idx: number) => {
        const curr = formData.keys ?? [];
        if (curr.length <= 1) return;
        const next = curr.filter((_, i) => i !== idx);
        onFormDataChange({ ...formData, keys: next });
    };

    const handleAddBaseUrl = () => {
        onFormDataChange({
            ...formData,
            base_urls: [...(formData.base_urls ?? []), { url: '', delay: 0, type: ChannelType.OpenAIChat }],
        });
    };

    const handleUpdateBaseUrl = (idx: number, patch: Partial<Channel['base_urls'][number]>) => {
        const next = (formData.base_urls ?? []).map((u, i) => (i === idx ? { ...u, ...patch } : u));
        onFormDataChange({ ...formData, base_urls: next });
    };

    const handleRemoveBaseUrl = (idx: number) => {
        const curr = formData.base_urls ?? [];
        if (curr.length <= 1) return;
        onFormDataChange({ ...formData, base_urls: curr.filter((_, i) => i !== idx) });
    };

    const handleAddHeader = () => {
        onFormDataChange({
            ...formData,
            custom_header: [...(formData.custom_header ?? []), { header_key: '', header_value: '' }],
        });
    };

    const handleUpdateHeader = (idx: number, patch: Partial<Channel['custom_header'][number]>) => {
        const next = (formData.custom_header ?? []).map((h, i) => (i === idx ? { ...h, ...patch } : h));
        onFormDataChange({ ...formData, custom_header: next });
    };

    const handleRemoveHeader = (idx: number) => {
        const curr = formData.custom_header ?? [];
        if (curr.length <= 1) return;
        onFormDataChange({ ...formData, custom_header: curr.filter((_, i) => i !== idx) });
    };

    const handleOAuthSuccess = (result: NonNullable<AuthResult['result']>) => {
        let channelKey: string;
        if (selectedProvider?.id === 'codex') {
            // 构建 Codex 完整凭证 JSON，包含 refresh_token、account_id 等
            const cred: Record<string, string> = {
                access_token: result.access_token,
                token_type: result.token_type || 'Bearer',
            };
            if (result.refresh_token) cred.refresh_token = result.refresh_token;
            if (result.expires_in) {
                cred.expires_at = new Date(Date.now() + result.expires_in * 1000).toISOString();
            }
            if (result.extra?.email) cred.email = result.extra.email;
            if (result.extra?.account_id) cred.account_id = result.extra.account_id;
            if (result.extra?.id_token) cred.id_token = result.extra.id_token;
            channelKey = JSON.stringify(cred);
        } else {
            channelKey = result.access_token;
        }
        // 添加新 OAuth 凭证到 keys 列表（remark 存邮箱标识）
        const oauthEmail = result.extra?.email || result.extra?.account_id || '';
        onFormDataChange({
            ...formData,
            keys: [...formData.keys.filter(k => k.channel_key.trim()), { enabled: true, channel_key: channelKey, remark: oauthEmail }],
        });
    };

    // 从 OAuth 凭证 JSON 中提取显示名（email > account_id）
    const getOAuthLabel = parseOAuthLabel;

    // provider_id → ChannelType 映射（用于按 provider 选择时设置 BaseUrl 的 type）
    const providerToTypeMap: Record<string, ChannelType> = {
        'openai-chat': ChannelType.OpenAIChat,
        'openai-response': ChannelType.OpenAIResponse,
        'anthropic': ChannelType.Anthropic,
        'gemini': ChannelType.Gemini,
        'volcengine': ChannelType.Volcengine,
        'openai-embedding': ChannelType.OpenAIEmbedding,
        'codex': ChannelType.OpenAIResponse,
    };

    // 为某个 BaseUrl 行选择渠道类型/提供商
    const handleBaseUrlKindChange = (idx: number, value: string) => {
        if (!value.startsWith('provider:')) return;
        const providerId = value.slice('provider:'.length);
        const provider = providers?.find((p) => p.id === providerId);
        const patch: Partial<Channel['base_urls'][number]> = { provider_id: providerId };
        if (provider) {
            patch.type = providerToTypeMap[provider.id] ?? ChannelType.OpenAIChat;
        }
        // 若 URL 为空且 provider 有默认 URL，自动填充
        const curr = formData.base_urls[idx];
        if (curr && curr.url.trim() === '') {
            const schema = provider?.credential_schema;
            const defaultUrl = schema?.fields?.find((f) => f.key === 'base_url')?.default;
            if (defaultUrl) {
                patch.url = defaultUrl;
            }
        }
        handleUpdateBaseUrl(idx, patch);
    };

    return (
        <form onSubmit={onSubmit} className="space-y-4 px-1">
            <div className="space-y-2">
                <label htmlFor={`${idPrefix}-name`} className="text-sm font-medium text-card-foreground">
                    {t('name')}
                </label>
                <Input
                    className='rounded-xl'
                    id={`${idPrefix}-name`}
                    type="text"
                    value={formData.name}
                    onChange={(event) => onFormDataChange({ ...formData, name: event.target.value })}
                    required
                />
            </div>

            <div className="space-y-2">
                <div className="flex items-center justify-between">
                    <label className="text-sm font-medium text-card-foreground">
                        {t('baseUrls')} {formData.base_urls.length > 0 ? `(${formData.base_urls.length})` : ''}
                    </label>
                    <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={handleAddBaseUrl}
                        className="h-6 px-2 text-xs text-muted-foreground/70 hover:text-muted-foreground hover:bg-transparent"
                    >
                        <Plus className="h-3 w-3 mr-1" />
                        {t('add')}
                    </Button>
                </div>
                <div className="space-y-2">
                    {(formData.base_urls ?? []).map((u, idx) => {
                        const buProviderValue = u.provider_id ? `provider:${u.provider_id}` : '';
                        return (
                            <div key={`baseurl-${idx}`} className="rounded-xl border border-border p-2 space-y-2">
                                <div className="flex items-center gap-2">
                                    <Input
                                        id={`${idPrefix}-base-${idx}`}
                                        type="url"
                                        value={u.url}
                                        onChange={(e) => handleUpdateBaseUrl(idx, { url: e.target.value })}
                                        placeholder={t('baseUrlUrl')}
                                        required={idx === 0}
                                        className="rounded-xl flex-1"
                                    />
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        size="sm"
                                        onClick={() => handleRemoveBaseUrl(idx)}
                                        disabled={(formData.base_urls ?? []).length <= 1}
                                        className="h-8 w-8 p-0 rounded-xl text-muted-foreground hover:text-destructive disabled:opacity-40 hover:bg-transparent"
                                        title="Remove"
                                    >
                                        <X className="h-4 w-4" />
                                    </Button>
                                </div>
                                <div className="flex items-center gap-2">
                                    <Select
                                        value={buProviderValue}
                                        onValueChange={(v) => handleBaseUrlKindChange(idx, v)}
                                    >
                                        <SelectTrigger id={`${idPrefix}-kind-${idx}`} className="rounded-xl flex-1 border border-border px-3 py-1.5 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                                            <SelectValue placeholder={t('type')} />
                                        </SelectTrigger>
                                        <SelectContent className='rounded-xl max-h-80'>
                                            {providers?.map((provider) => (
                                                <SelectItem key={provider.id} className='rounded-xl' value={`provider:${provider.id}`}>
                                                    {provider.display_name}
                                                </SelectItem>
                                            ))}
                                        </SelectContent>
                                    </Select>
                                </div>
                            </div>
                        );
                    })}
                </div>
            </div>

            <div className="space-y-2">
                <div className="flex items-center justify-between">
                    <label className="text-sm font-medium text-card-foreground">
                        {isOAuthProvider ? t('oauthCredential') : t('apiKey')} {formData.keys.length > 0 ? `(${formData.keys.length})` : ''}
                    </label>
                    {!isOAuthProvider && (
                        <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={handleAddKey}
                            className="h-6 px-2 text-xs text-muted-foreground/70 hover:text-muted-foreground hover:bg-transparent"
                        >
                            <Plus className="h-3 w-3 mr-1" />
                            {t('add')}
                        </Button>
                    )}
                </div>
                {isOAuthProvider ? (
                    <div className="space-y-2">
                        {formData.keys.filter(k => k.channel_key.trim()).map((key, idx) => {
                            const label = getOAuthLabel(key.channel_key);
                            // 解析凭证 JSON 提取 account_id 和 expires_at
                            let accountId = '';
                            let expiresAt = '';
                            try {
                                const parsed = JSON.parse(key.channel_key);
                                if (parsed && typeof parsed === 'object') {
                                    accountId = parsed.account_id || '';
                                    if (parsed.expires_at) {
                                        try {
                                            const d = new Date(parsed.expires_at);
                                            if (!isNaN(d.getTime())) {
                                                expiresAt = d.toLocaleDateString();
                                            }
                                        } catch { /* ignore */ }
                                    }
                                }
                            } catch { /* not JSON */ }
                            const secondaryParts = [accountId, expiresAt].filter(Boolean);
                            return (
                                <div key={idx} className="flex items-center justify-between rounded-xl border border-border px-3 py-2">
                                    <div className="flex items-center gap-2 min-w-0">
                                        <Switch
                                            checked={key.enabled}
                                            onCheckedChange={(checked) => {
                                                const filled = formData.keys.filter(k => k.channel_key.trim());
                                                const realIdx = formData.keys.indexOf(key);
                                                if (realIdx >= 0) {
                                                    handleUpdateKey(realIdx, { enabled: checked });
                                                }
                                            }}
                                            className="shrink-0 scale-75"
                                        />
                                        <Badge variant="outline" className="text-[10px] px-1.5 py-0 shrink-0">Codex</Badge>
                                        <div className="min-w-0">
                                            <span className="text-sm text-muted-foreground truncate block">{key.remark || label || `${t('oauthAuthorized')} #${idx + 1}`}</span>
                                            {secondaryParts.length > 0 && (
                                                <span className="text-xs text-muted-foreground/60 truncate block">{secondaryParts.join(' · ')}</span>
                                            )}
                                        </div>
                                    </div>
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        size="sm"
                                        className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive shrink-0"
                                        onClick={() => {
                                            const filled = formData.keys.filter(k => k.channel_key.trim());
                                            filled.splice(idx, 1);
                                            onFormDataChange({ ...formData, keys: filled.length ? filled : [{ enabled: true, channel_key: '', remark: '' }] });
                                        }}
                                    >
                                        <X className="h-3.5 w-3.5" />
                                    </Button>
                                </div>
                            );
                        })}
                        <OAuthPanel
                            providerId={selectedProvider?.id || ''}
                            authType={selectedProvider?.auth_type as 'oauth_device' | 'oauth_web'}
                            onSuccess={handleOAuthSuccess}
                        />
                        {selectedProvider?.id === 'codex' && (
                            <AuthFileImportPanel
                                channelId={channelId}
                                onImportComplete={async () => {
                                    // 导入完成后从服务器重新获取渠道数据，更新表单中的 keys
                                    try {
                                        const { apiClient } = await import('@/api/client');
                                        type ChannelKeyBrief = { id: number; channel_key: string; remark: string; enabled: boolean };
                                        type ChannelBrief = { id: number; keys: ChannelKeyBrief[] };
                                        const channels = await apiClient.get<ChannelBrief[]>('/api/v1/channel/list');
                                        const updated = channels.find((c) => c.id === channelId);
                                        if (updated && updated.keys) {
                                            const newKeys = updated.keys.length > 0
                                                ? updated.keys.map((k) => ({
                                                    id: k.id,
                                                    enabled: k.enabled,
                                                    channel_key: k.channel_key,
                                                    remark: k.remark ?? '',
                                                }))
                                                : [{ enabled: true, channel_key: '', remark: '' }];
                                            onFormDataChange({ ...formData, keys: newKeys });
                                        }
                                    } catch { /* 静默失败，用户可手动刷新 */ }
                                }}
                            />
                        )}
                    </div>
                ) : (
                    <div className="space-y-2">
                        {(formData.keys ?? []).map((k, idx) => (
                            <div key={k.id ?? `new-${idx}`} className="flex items-center gap-2">
                                <Input
                                    type="text"
                                    value={k.channel_key}
                                    onChange={(e) => handleUpdateKey(idx, { channel_key: e.target.value })}
                                    placeholder={t('apiKey')}
                                    required={idx === 0}
                                    className="rounded-xl flex-1"
                                />
                                <Input
                                    type="text"
                                    value={k.remark ?? ''}
                                    onChange={(e) => handleUpdateKey(idx, { remark: e.target.value })}
                                    placeholder={t('remark')}
                                    className="rounded-xl w-32"
                                />
                                <Switch
                                    checked={k.enabled}
                                    onCheckedChange={(checked) => handleUpdateKey(idx, { enabled: checked })}
                                />
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => handleRemoveKey(idx)}
                                    disabled={(formData.keys ?? []).length <= 1}
                                    className="h-8 w-8 p-0 rounded-xl text-muted-foreground hover:text-destructive hover:bg-transparent disabled:opacity-40"
                                    title="Remove"
                                >
                                    <X className="h-4 w-4" />
                                </Button>
                            </div>
                        ))}
                    </div>
                )}
            </div>

            <div className="space-y-2">
                <div className="flex items-center justify-between">
                    <label className="text-sm font-medium text-card-foreground">{t('model')}</label>
                    <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={handleRefreshModels}
                        disabled={!formData.base_urls?.[0]?.url || !effectiveKey || fetchModel.isPending}
                        className="h-6 px-2 text-xs text-muted-foreground/50 hover:text-muted-foreground hover:bg-transparent"
                    >
                        <RefreshCw className={`h-3 w-3 mr-1 ${fetchModel.isPending ? 'animate-spin' : ''}`} />
                        {t('modelRefresh')}
                    </Button>
                </div>
                <input type="hidden" value={formData.model} required />

                <div className="relative">
                    <Input
                        ref={inputRef}
                        id={`${idPrefix}-model-custom`}
                        type="text"
                        value={inputValue}
                        onChange={(e) => setInputValue(e.target.value)}
                        onKeyDown={handleInputKeyDown}
                        placeholder={t('modelCustomPlaceholder')}
                        className="pr-10 rounded-xl"
                    />
                    {inputValue.trim() && !customModels.includes(inputValue.trim()) && !autoModels.includes(inputValue.trim()) && (
                        <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => handleAddModel(inputValue)}
                            className="absolute rounded-lg right-1 top-1/2 -translate-y-1/2 h-7 w-7 p-0 text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
                            title={t('modelAdd')}
                        >
                            <Plus className="size-4" />
                        </Button>
                    )}
                </div>

                <div className="space-y-2">
                    <div className="flex items-center justify-between">
                        <label className="text-xs font-medium text-card-foreground">
                            {t('modelSelected')} {(autoModels.length + customModels.length) > 0 && `(${autoModels.length + customModels.length})`}
                        </label>
                        {(autoModels.length + customModels.length) > 0 && (
                            <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={() => {
                                    updateModels([], []);
                                }}
                                className="h-6 px-2 text-xs text-muted-foreground/50 hover:text-muted-foreground hover:bg-transparent"
                            >
                                {t('modelClearAll')}
                            </Button>
                        )}
                    </div>
                    <div className="rounded-xl border border-border bg-muted/30 p-2.5 max-h-40 min-h-12 overflow-y-auto">
                        {(autoModels.length + customModels.length) > 0 ? (
                            <div className="flex flex-wrap gap-1.5">
                                {autoModels.map((model) => (
                                    <Badge key={model} variant="secondary" className="bg-muted hover:bg-muted/80">
                                        {model}
                                        <button
                                            type="button"
                                            onClick={() => handleRemoveAutoModel(model)}
                                            className="ml-1 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-ring"
                                        >
                                            <X className="h-3 w-3" />
                                        </button>
                                    </Badge>
                                ))}
                                {customModels.map((model) => (
                                    <Badge key={model} className="bg-primary hover:bg-primary/90">
                                        {model}
                                        <button
                                            type="button"
                                            onClick={() => handleRemoveCustomModel(model)}
                                            className="ml-1 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-ring"
                                        >
                                            <X className="h-3 w-3" />
                                        </button>
                                    </Badge>
                                ))}
                            </div>
                        ) : (
                            <div className="flex items-center justify-center h-8 text-xs text-muted-foreground">
                                {t('modelNoSelected')}
                            </div>
                        )}
                    </div>
                </div>
            </div>

            {/* Model test buttons */}
            {allModels.length > 0 && (
                <div className="space-y-2">
                    <div className="flex items-center gap-2">
                        <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={handleTestFirst}
                            disabled={testModels.isPending}
                            className="rounded-xl text-xs"
                        >
                            {testModels.isPending ? <Loader2 className="h-3 w-3 mr-1 animate-spin" /> : <CheckCircle2 className="h-3 w-3 mr-1" />}
                            {t('testFirst')}
                        </Button>
                        <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            onClick={handleTestAll}
                            disabled={testModels.isPending}
                            className="rounded-xl text-xs"
                        >
                            {testModels.isPending ? <Loader2 className="h-3 w-3 mr-1 animate-spin" /> : <CheckCircle2 className="h-3 w-3 mr-1" />}
                            {t('testAll')}
                        </Button>
                    </div>
                    {testResults.length > 0 && (
                        <div className="space-y-1">
                            <label className="text-xs font-medium text-card-foreground">{t('testResult')}</label>
                            <div className="rounded-xl border border-border bg-muted/30 p-2 space-y-1 max-h-40 overflow-y-auto">
                                {testResults.map((r) => (
                                    <div key={r.model} className="flex items-center justify-between text-xs px-1 py-0.5">
                                        <div className="flex items-center gap-1.5 min-w-0">
                                            {r.passed ? (
                                                <CheckCircle2 className="h-3 w-3 shrink-0 text-green-600 dark:text-green-400" />
                                            ) : (
                                                <XCircle className="h-3 w-3 shrink-0 text-red-600 dark:text-red-400" />
                                            )}
                                            <span className="truncate">{r.model}</span>
                                        </div>
                                        <div className="flex items-center gap-2 shrink-0">
                                            {r.error && (
                                                <span className="text-muted-foreground truncate max-w-48" title={r.error}>{r.error}</span>
                                            )}
                                            {r.delay != null && (
                                                <span className="text-muted-foreground">{r.delay}ms</span>
                                            )}
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}
                </div>
            )}

            {/* Model selection dialog */}
            <Dialog open={showModelDialog} onOpenChange={setShowModelDialog}>
                <DialogContent className="sm:max-w-md max-h-[80vh] flex flex-col">
                    <DialogHeader>
                        <DialogTitle>{t('modelDialogTitle')}</DialogTitle>
                        <DialogDescription>
                            {formData.auto_sync && (
                                <span className="flex items-center gap-1 text-amber-600 dark:text-amber-400">
                                    <AlertTriangle className="h-3 w-3" />
                                    {t('modelDialogAutoSyncWarning')}
                                </span>
                            )}
                        </DialogDescription>
                    </DialogHeader>

                    <div className="space-y-3 flex-1 min-h-0 flex flex-col">
                        <Input
                            type="text"
                            value={dialogSearch}
                            onChange={(e) => setDialogSearch(e.target.value)}
                            placeholder={t('modelDialogSearch')}
                            className="rounded-xl"
                        />
                        <div className="flex items-center gap-2">
                            <Checkbox
                                id="dialog-select-all"
                                checked={dialogAllSelected}
                                onCheckedChange={handleDialogToggleAll}
                            />
                            <label htmlFor="dialog-select-all" className="text-xs text-muted-foreground cursor-pointer">
                                {dialogAllSelected ? t('modelDialogDeselectAll') : t('modelDialogSelectAll')}
                            </label>
                        </div>
                        <div className="flex-1 overflow-y-auto border rounded-xl p-2 space-y-1 min-h-0 max-h-60">
                            {filteredDialogModels.length === 0 ? (
                                <div className="flex items-center justify-center h-12 text-xs text-muted-foreground">
                                    {t('modelRefreshEmpty')}
                                </div>
                            ) : (
                                filteredDialogModels.map((model) => (
                                    <label key={model} className="flex items-center gap-2 px-2 py-1.5 rounded-lg hover:bg-muted/50 cursor-pointer">
                                        <Checkbox
                                            checked={dialogSelectedModels.has(model)}
                                            onCheckedChange={() => handleDialogToggleModel(model)}
                                        />
                                        <span className="text-sm truncate">{model}</span>
                                    </label>
                                ))
                            )}
                        </div>
                    </div>

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="outline"
                            onClick={() => setShowModelDialog(false)}
                            className="rounded-xl"
                        >
                            {t('modelDialogCancel')}
                        </Button>
                        <Button
                            type="button"
                            onClick={handleDialogConfirm}
                            className="rounded-xl"
                        >
                            {t('modelDialogConfirm')} ({dialogSelectedModels.size})
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <Accordion type="single" collapsible className="w-full border rounded-xl bg-card">
                <AccordionItem value="advanced" className="border-none">
                    <AccordionTrigger className="text-sm font-medium text-card-foreground py-3 px-4 hover:no-underline hover:bg-muted/30 rounded-xl transition-colors">
                        {t('advanced')}
                    </AccordionTrigger>
                    <AccordionContent className="pt-4 px-4 pb-4 space-y-4 border-t">
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <div className="space-y-2">
                                <label htmlFor={`${idPrefix}-auto-group`} className="text-sm font-medium text-card-foreground">
                                    {t('autoGroup')}
                                </label>
                                <Select
                                    value={String(formData.auto_group)}
                                    onValueChange={(value) => onFormDataChange({ ...formData, auto_group: Number(value) as AutoGroupType })}
                                >
                                    <SelectTrigger id={`${idPrefix}-auto-group`} className="rounded-xl w-full border border-border px-4 py-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                                        <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent className='rounded-xl'>
                                        <SelectItem className='rounded-xl' value={String(AutoGroupType.None)}>{t('autoGroupNone')}</SelectItem>
                                        <SelectItem className='rounded-xl' value={String(AutoGroupType.Fuzzy)}>{t('autoGroupFuzzy')}</SelectItem>
                                        <SelectItem className='rounded-xl' value={String(AutoGroupType.Exact)}>{t('autoGroupExact')}</SelectItem>
                                        <SelectItem className='rounded-xl' value={String(AutoGroupType.Regex)}>{t('autoGroupRegex')}</SelectItem>
                                    </SelectContent>
                                </Select>
                            </div>

                            <div className="space-y-2">
                                <label htmlFor={`${idPrefix}-channel-proxy`} className="text-sm font-medium text-card-foreground">
                                    {t('channelProxy')}
                                </label>
                                <Input
                                    id={`${idPrefix}-channel-proxy`}
                                    type="text"
                                    value={formData.channel_proxy}
                                    onChange={(e) => onFormDataChange({ ...formData, channel_proxy: e.target.value })}
                                    placeholder={t('channelProxyPlaceholder')}
                                    className="rounded-xl"
                                />
                            </div>
                        </div>

                        <div className="space-y-2">
                            <div className="flex items-center justify-between">
                                <label className="text-sm font-medium text-card-foreground">
                                    {t('customHeader')} {formData.custom_header.length > 0 ? `(${formData.custom_header.length})` : ''}
                                </label>
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={handleAddHeader}
                                    className="h-6 px-2 text-xs text-muted-foreground/70 hover:text-muted-foreground hover:bg-transparent"
                                >
                                    <Plus className="h-3 w-3 mr-1" />
                                    {t('customHeaderAdd')}
                                </Button>
                            </div>
                            <div className="space-y-2">
                                {(formData.custom_header ?? []).map((h, idx) => (
                                    <div key={`hdr-${idx}`} className="flex items-center gap-2">
                                        <Input
                                            type="text"
                                            value={h.header_key}
                                            onChange={(e) => handleUpdateHeader(idx, { header_key: e.target.value })}
                                            placeholder={t('customHeaderKey')}
                                            className="rounded-xl flex-1"
                                        />
                                        <Input
                                            type="text"
                                            value={h.header_value}
                                            onChange={(e) => handleUpdateHeader(idx, { header_value: e.target.value })}
                                            placeholder={t('customHeaderValue')}
                                            className="rounded-xl flex-1"
                                        />
                                        <Button
                                            type="button"
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => handleRemoveHeader(idx)}
                                            disabled={(formData.custom_header ?? []).length <= 1}
                                            className="h-8 w-8 p-0 rounded-xl text-muted-foreground hover:text-destructive hover:bg-transparent disabled:opacity-40"
                                            title="Remove"
                                        >
                                            <X className="h-4 w-4" />
                                        </Button>
                                    </div>
                                ))}
                            </div>
                        </div>

                        <div className="space-y-2">
                            <label htmlFor={`${idPrefix}-match-regex`} className="text-sm font-medium text-card-foreground">
                                {t('matchRegex')}
                            </label>
                            <Input
                                id={`${idPrefix}-match-regex`}
                                type="text"
                                value={formData.match_regex}
                                onChange={(e) => onFormDataChange({ ...formData, match_regex: e.target.value })}
                                placeholder={t('matchRegexPlaceholder')}
                                className="rounded-xl"
                            />
                        </div>

                        <div className="space-y-2">
                            <label htmlFor={`${idPrefix}-param-override`} className="text-sm font-medium text-card-foreground">
                                {t('paramOverride')}
                            </label>
                            <textarea
                                id={`${idPrefix}-param-override`}
                                value={formData.param_override}
                                onChange={(e) => onFormDataChange({ ...formData, param_override: e.target.value })}
                                placeholder={t('paramOverridePlaceholder')}
                                className="min-h-28 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            />
                        </div>
                    </AccordionContent>
                </AccordionItem>
            </Accordion>

            <div className="flex flex-wrap items-center justify-between gap-4 p-4 rounded-xl bg-muted/20 border border-border/50">
                <label className="flex items-center gap-2 cursor-pointer">
                    <Switch
                        checked={formData.enabled}
                        onCheckedChange={(checked) => onFormDataChange({ ...formData, enabled: checked })}
                    />
                    <span className="text-sm font-medium text-card-foreground">{t('enabled')}</span>
                </label>
                <div className="flex items-center gap-6">
                    <label className="flex items-center gap-2 cursor-pointer">
                        <Switch
                            checked={formData.proxy}
                            onCheckedChange={(checked) => onFormDataChange({ ...formData, proxy: checked })}
                        />
                        <span className="text-sm text-card-foreground">{t('proxy')}</span>
                    </label>
                    <label className="flex items-center gap-2 cursor-pointer">
                        <Switch
                            checked={formData.auto_sync}
                            onCheckedChange={(checked) => onFormDataChange({ ...formData, auto_sync: checked })}
                        />
                        <span className="text-sm text-card-foreground">{t('autoSync')}</span>
                    </label>
                </div>
            </div>

            <div className={`flex flex-col gap-3 pt-2 ${onCancel ? 'sm:flex-row' : ''}`}>
                {onCancel && cancelText && (
                    <Button
                        type="button"
                        variant="secondary"
                        onClick={onCancel}
                        className="w-full sm:flex-1 rounded-2xl h-12"
                    >
                        {cancelText}
                    </Button>
                )}
                {!hideSubmit && (
                <Button
                    type="submit"
                    disabled={isPending}
                    className="w-full sm:flex-1 rounded-2xl h-12"
                >
                    {isPending ? pendingText : submitText}
                </Button>
                )}
            </div>
        </form>
    );
}
