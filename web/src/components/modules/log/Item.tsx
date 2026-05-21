'use client';

import { useMemo, useState, useEffect, useRef } from 'react';
import { Clock, Cpu, Zap, AlertCircle, ArrowDownToLine, ArrowUpFromLine, DollarSign, ArrowRight, ArrowDown, Send, MessageSquare, Loader2, RotateCw, ChevronDown, ChevronUp, Pin, KeyRound, Gauge, Brain, Database } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { motion, AnimatePresence } from 'motion/react';
import JsonView from '@uiw/react-json-view';
import { githubDarkTheme } from '@uiw/react-json-view/githubDark';
import { githubLightTheme } from '@uiw/react-json-view/githubLight';
import { useTheme } from 'next-themes';
import { type RelayLog, type ChannelAttempt, getLogDetail } from '@/api/endpoints/log';
import { getModelIcon } from '@/lib/model-icons';
import { ClientIconBadge } from './ClientIcon';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { CopyIconButton } from '@/components/common/CopyButton';
import {
    MorphingDialog,
    MorphingDialogTrigger,
    MorphingDialogContainer,
    MorphingDialogContent,
    MorphingDialogClose,
    MorphingDialogTitle,
    MorphingDialogDescription,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { Tooltip, TooltipContent, TooltipTrigger, TooltipProvider } from '@/components/animate-ui/components/animate/tooltip';

function formatTime(timestamp: number): string {
    const date = new Date(timestamp * 1000);
    return date.toLocaleString('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
    });
}

function formatDuration(ms: number): string {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
}

// 格式化大数字：>= 1000 用 K，>= 1000000 用 M
function formatTokenCount(n: number): string {
    if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
    if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
    return n.toString();
}

const REASONING_EFFORT_COLORS: Record<string, string> = {
    low: '#6b7280',
    medium: '#f59e0b',
    high: '#8b5cf6',
    xhigh: '#e11d48',
    max: '#dc2626',
};

function ReasoningEffortBadge({ effort }: { effort: string }) {
    const color = REASONING_EFFORT_COLORS[effort] ?? '#6b7280';
    return (
        <Badge
            variant="secondary"
            className="shrink-0 text-xs px-1.5 py-0"
            style={{ backgroundColor: `${color}15`, color }}
        >
            <Brain className="size-3 mr-1 opacity-80" />
            {effort}
        </Badge>
    );
}

interface RetryBadgeWithTooltipProps {
    channelName: string;
    brandColor: string;
    attempts: ChannelAttempt[];
}

function RetryBadgeWithTooltip({ channelName, brandColor, attempts }: RetryBadgeWithTooltipProps) {
    const t = useTranslations('log.card');

    return (
        <Tooltip>
            <TooltipTrigger asChild>
                <Badge
                    variant="secondary"
                    className="shrink-0 text-xs px-1.5 py-0 cursor-help"
                    style={{ backgroundColor: `${brandColor}15`, color: brandColor }}
                >
                    <RotateCw className="size-3 mr-1 opacity-80" />
                    {channelName}
                </Badge>
            </TooltipTrigger>
            <TooltipContent className="border bg-card p-2 min-w-[280px] shadow-sm rounded-3xl flex flex-col gap-1">
                {attempts.map((attempt, idx) => (
                    <div key={idx} className="flex flex-col w-full">
                        <div className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted/50 transition-colors">
                            <Badge
                                className={cn(
                                    "h-5 shrink-0 px-1.5 text-[10px] font-bold uppercase shadow-none border-0",
                                    attempt.status === 'success'
                                        ? "bg-primary/15 text-primary"
                                        : "bg-destructive/15 text-destructive"
                                )}
                            >
                                {attempt.status === 'success' ? t('success') : t('failed')}
                            </Badge>
                            <div className="flex min-w-0 flex-col flex-1">
                                <span className="truncate text-xs font-semibold text-foreground">
                                    {attempt.channel_key_remark ? `${attempt.channel_name}-${attempt.channel_key_remark}` : attempt.channel_name}
                                </span>
                                <span className="text-[10px] text-muted-foreground">
                                    {attempt.model_name} • {formatDuration(attempt.duration)}
                                </span>
                            </div>
                        </div>
                        {
                            idx < attempts.length - 1 && (
                                <div className="flex justify-center py-0.5">
                                    <ArrowDown className="size-3 text-muted-foreground/30" />
                                </div>
                            )
                        }
                    </div>
                ))}
            </TooltipContent>
        </Tooltip >
    );
}

function DeferredJsonContent({ content, fallbackText, loading }: { content: string | undefined; fallbackText: string; loading?: boolean }) {
    const { resolvedTheme } = useTheme();
    const { isOpen } = useMorphingDialog();
    const [shouldRender, setShouldRender] = useState(false);

    const parsed = useMemo(() => {
        if (!content) return { isJson: false, data: null };
        try {
            return { isJson: true, data: JSON.parse(content) };
        } catch {
            return { isJson: false, data: content };
        }
    }, [content]);

    useEffect(() => {
        if (isOpen) {
            const timer = setTimeout(() => setShouldRender(true), 300);
            return () => clearTimeout(timer);
        }
    }, [isOpen]);

    if (!isOpen) {
        if (shouldRender) setShouldRender(false);
        return null;
    }

    if (loading) {
        return (
            <div className="p-4 flex items-center justify-center h-full">
                <Loader2 className="h-5 w-5 text-muted-foreground animate-spin" />
            </div>
        );
    }

    if (!content) {
        return (
            <pre className="p-4 text-xs text-muted-foreground whitespace-pre-wrap wrap-break-word leading-relaxed">
                {fallbackText}
            </pre>
        );
    }

    return (
        <AnimatePresence mode="wait">
            {!shouldRender ? (
                <motion.div
                    key="loading"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.15 }}
                    className="p-4 flex items-center justify-center h-full"
                >
                    <Loader2 className="h-5 w-5 text-muted-foreground animate-spin" />
                </motion.div>
            ) : parsed.isJson ? (
                <motion.div
                    key="json"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.2 }}
                    className="p-4"
                >
                    <JsonView
                        value={parsed.data as object}
                        style={{
                            ...(resolvedTheme === 'dark' ? githubDarkTheme : githubLightTheme),
                            fontSize: '12px',
                            fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
                            backgroundColor: 'transparent',
                        }}
                        displayDataTypes={false}
                        displayObjectSize={false}
                        collapsed={false}
                    />
                </motion.div>
            ) : (
                <motion.pre
                    key="text"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.2 }}
                    className="p-4 text-xs text-muted-foreground whitespace-pre-wrap wrap-break-word font-mono leading-relaxed"
                >
                    {content}
                </motion.pre>
            )}
        </AnimatePresence>
    );
}

type ParsedResponseLog = {
    text: string;
    thinking: string;
    toolCalls: string[];
    meta: string[];
    hasSummary: boolean;
    isJson: boolean;
    jsonData: unknown;
};

type JsonRecord = Record<string, unknown>;

const MAX_SUMMARY_CHARS = 60000;
const MAX_SSE_EVENTS = 2000;

function appendLimited(current: string, value?: string): string {
    if (!value) return current;
    if (current.length >= MAX_SUMMARY_CHARS) return current;
    const next = current + value;
    return next.length > MAX_SUMMARY_CHARS ? `${next.slice(0, MAX_SUMMARY_CHARS)}\n...[truncated]` : next;
}

function parseSSE(content: string): Array<{ event?: string; data: string }> {
    const events: Array<{ event?: string; data: string }> = [];
    const blocks = content.split(/\n\s*\n/);
    for (const block of blocks) {
        if (events.length >= MAX_SSE_EVENTS) break;
        const lines = block.split(/\r?\n/);
        let event: string | undefined;
        const data: string[] = [];
        for (const line of lines) {
            if (line.startsWith('event:')) event = line.slice(6).trim();
            if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
        }
        if (data.length > 0) events.push({ event, data: data.join('\n') });
    }
    return events;
}

function extractTextFromUnknown(value: unknown): string {
    if (typeof value === 'string') return value;
    if (!value || typeof value !== 'object') return '';
    if (Array.isArray(value)) return value.map(extractTextFromUnknown).filter(Boolean).join('');

    const obj = value as Record<string, unknown>;
    if (typeof obj.text === 'string') return obj.text;
    if (typeof obj.content === 'string') return obj.content;
    if (Array.isArray(obj.content)) return obj.content.map(extractTextFromUnknown).filter(Boolean).join('');
    if (Array.isArray(obj.parts)) return obj.parts.map(extractTextFromUnknown).filter(Boolean).join('');
    if (Array.isArray(obj.output)) return obj.output.map(extractTextFromUnknown).filter(Boolean).join('');
    if (obj.message) return extractTextFromUnknown(obj.message);
    return '';
}

function parseResponseEvent(data: unknown, acc: ParsedResponseLog) {
    if (!data || typeof data !== 'object') return;
    const obj = data as JsonRecord;
    const type = typeof obj.type === 'string' ? obj.type : undefined;

    // OpenAI Responses / Anthropic style event type
    if (type === 'response.output_text.delta' && typeof obj.delta === 'string') {
        acc.text = appendLimited(acc.text, obj.delta);
    } else if (type === 'response.reasoning_summary_text.delta' && typeof obj.delta === 'string') {
        acc.thinking = appendLimited(acc.thinking, obj.delta);
    } else if (type === 'response.completed' && obj.response && typeof obj.response === 'object') {
        const response = obj.response as JsonRecord;
        acc.text = appendLimited(acc.text, extractTextFromUnknown(response.output));
        if (response.status) acc.meta.push(`status: ${String(response.status)}`);
    } else if (type === 'response.output_item.added' && obj.item && typeof obj.item === 'object' && (obj.item as JsonRecord).type === 'function_call') {
        const item = obj.item as JsonRecord;
        acc.toolCalls.push(`${String(item.name ?? 'function')}(${String(item.arguments ?? '')})`);
    } else if (type === 'response.function_call_arguments.delta' && typeof obj.delta === 'string') {
        acc.toolCalls.push(`arguments delta: ${obj.delta}`);
    } else if (type === 'content_block_delta' && obj.delta && typeof obj.delta === 'object') {
        const delta = obj.delta as JsonRecord;
        if (delta.type === 'text_delta') acc.text = appendLimited(acc.text, typeof delta.text === 'string' ? delta.text : undefined);
        if (delta.type === 'thinking_delta') acc.thinking = appendLimited(acc.thinking, typeof delta.thinking === 'string' ? delta.thinking : undefined);
        if (delta.type === 'input_json_delta') acc.toolCalls.push(`input delta: ${String(delta.partial_json ?? '')}`);
    } else if (type === 'content_block_start' && obj.content_block && typeof obj.content_block === 'object' && (obj.content_block as JsonRecord).type === 'tool_use') {
        const block = obj.content_block as JsonRecord;
        acc.toolCalls.push(`${String(block.name ?? 'tool')}(${JSON.stringify(block.input ?? {})})`);
    } else if (type === 'message_delta') {
        if (obj.delta && typeof obj.delta === 'object' && (obj.delta as JsonRecord).stop_reason) {
            acc.meta.push(`stop: ${String((obj.delta as JsonRecord).stop_reason)}`);
        }
    }

    // OpenAI Chat Completions stream
    if (Array.isArray(obj.choices)) {
        for (const choice of obj.choices) {
            if (!choice || typeof choice !== 'object') continue;
            const choiceObj = choice as JsonRecord;
            const delta = choiceObj.delta && typeof choiceObj.delta === 'object' ? choiceObj.delta as JsonRecord : {};
            const message = choiceObj.message && typeof choiceObj.message === 'object' ? choiceObj.message as JsonRecord : {};
            acc.text = appendLimited(acc.text, extractTextFromUnknown(delta.content ?? message.content));
            const thinking = delta.reasoning_content ?? delta.reasoning ?? message.reasoning_content ?? message.reasoning;
            acc.thinking = appendLimited(acc.thinking, typeof thinking === 'string' ? thinking : undefined);
            const toolCalls = delta.tool_calls ?? message.tool_calls;
            if (Array.isArray(toolCalls)) {
                for (const call of toolCalls) {
                    if (!call || typeof call !== 'object') continue;
                    const callObj = call as JsonRecord;
                    const fn = callObj.function && typeof callObj.function === 'object' ? callObj.function as JsonRecord : {};
                    acc.toolCalls.push(`${String(fn.name ?? callObj.id ?? 'tool')}(${String(fn.arguments ?? '')})`);
                }
            }
            if (choiceObj.finish_reason) acc.meta.push(`finish: ${String(choiceObj.finish_reason)}`);
        }
    }

    // Gemini stream / JSON
    if (Array.isArray(obj.candidates)) {
        for (const candidate of obj.candidates) {
            if (!candidate || typeof candidate !== 'object') continue;
            const candidateObj = candidate as JsonRecord;
            const content = candidateObj.content && typeof candidateObj.content === 'object' ? candidateObj.content as JsonRecord : {};
            acc.text = appendLimited(acc.text, extractTextFromUnknown(content.parts));
            if (candidateObj.finishReason) acc.meta.push(`finish: ${String(candidateObj.finishReason)}`);
        }
    }

    acc.text = appendLimited(acc.text, extractTextFromUnknown(obj.content));
}

function parseResponseLog(content: string | undefined): ParsedResponseLog {
    const parsed: ParsedResponseLog = {
        text: '',
        thinking: '',
        toolCalls: [],
        meta: [],
        hasSummary: false,
        isJson: false,
        jsonData: null,
    };
    if (!content) return parsed;

    const looksLikeSSE = /(^|\n)(event|data):/.test(content);
    if (looksLikeSSE) {
        for (const event of parseSSE(content)) {
            if (event.data === '[DONE]') continue;
            try {
                parseResponseEvent(JSON.parse(event.data), parsed);
            } catch {
                // Keep raw fallback.
            }
        }
    } else {
        try {
            parsed.jsonData = JSON.parse(content);
            parsed.isJson = true;
            parseResponseEvent(parsed.jsonData, parsed);
            parsed.text = appendLimited(parsed.text, extractTextFromUnknown(parsed.jsonData));
        } catch {
            parsed.jsonData = content;
        }
    }

    parsed.toolCalls = [...new Set(parsed.toolCalls.filter(Boolean))].slice(0, 20);
    parsed.meta = [...new Set(parsed.meta.filter(Boolean))].slice(0, 20);
    parsed.hasSummary = Boolean(parsed.text.trim() || parsed.thinking.trim() || parsed.toolCalls.length || parsed.meta.length);
    return parsed;
}

function ResponseLogContent({ content, fallbackText, loading }: { content: string | undefined; fallbackText: string; loading?: boolean }) {
    const [rawOpen, setRawOpen] = useState(false);
    const parsed = useMemo(() => parseResponseLog(content), [content]);

    if (loading || !content || !parsed.hasSummary) {
        return <DeferredJsonContent content={content} fallbackText={fallbackText} loading={loading} />;
    }

    return (
        <div className="p-4 space-y-3 text-xs">
            {parsed.text.trim() && (
                <section>
                    <div className="mb-1 font-medium text-card-foreground">摘要</div>
                    <pre className="whitespace-pre-wrap wrap-break-word leading-relaxed text-muted-foreground font-sans">
                        {parsed.text.trim()}
                    </pre>
                </section>
            )}
            {parsed.thinking.trim() && (
                <details className="rounded-xl border border-border bg-background/50 p-3">
                    <summary className="cursor-pointer font-medium text-card-foreground">思考内容</summary>
                    <pre className="mt-2 whitespace-pre-wrap wrap-break-word leading-relaxed text-muted-foreground font-sans">
                        {parsed.thinking.trim()}
                    </pre>
                </details>
            )}
            {parsed.toolCalls.length > 0 && (
                <section>
                    <div className="mb-1 font-medium text-card-foreground">工具调用</div>
                    <div className="space-y-1">
                        {parsed.toolCalls.map((tool, idx) => (
                            <pre key={`${tool}-${idx}`} className="rounded-lg bg-background/70 px-2 py-1 font-mono text-muted-foreground whitespace-pre-wrap wrap-break-word">
                                {tool}
                            </pre>
                        ))}
                    </div>
                </section>
            )}
            {parsed.meta.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                    {parsed.meta.map((item) => (
                        <Badge key={item} variant="secondary" className="text-[10px]">
                            {item}
                        </Badge>
                    ))}
                </div>
            )}
            <button
                type="button"
                onClick={() => setRawOpen(v => !v)}
                className="text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
                {rawOpen ? '隐藏原始响应' : '查看原始响应'}
            </button>
            {rawOpen && (
                <pre className="rounded-xl border border-border bg-background/70 p-3 text-xs text-muted-foreground whitespace-pre-wrap wrap-break-word font-mono leading-relaxed">
                    {content}
                </pre>
            )}
        </div>
    );
}

// 必须在 MorphingDialog 内部使用，才能拿到 useMorphingDialog 的 context
// 负责懒加载 request_content / response_content 并渲染双栏内容
function LogDetailPanels({ log }: { log: RelayLog }) {
    const t = useTranslations('log.card');
    const { isOpen } = useMorphingDialog();
    const [detail, setDetail] = useState<RelayLog | null>(null);
    const [detailLoading, setDetailLoading] = useState(false);
    const [debugExpanded, setDebugExpanded] = useState(false);
    const loadedLogId = useRef<number | null>(null);

    useEffect(() => {
        let cancelled = false;
        if (isOpen && loadedLogId.current !== log.id) {
            queueMicrotask(() => {
                if (cancelled) return;
                loadedLogId.current = log.id;
                setDetail(null);
                setDetailLoading(true);
                getLogDetail(log.id)
                    .then((data) => {
                        if (!cancelled) setDetail(data ?? null);
                    })
                    .catch(() => {
                        if (!cancelled) setDetail(null);
                    })
                    .finally(() => {
                        if (!cancelled) setDetailLoading(false);
                    });
            });
        }
        if (!isOpen) {
            queueMicrotask(() => {
                if (cancelled) return;
                loadedLogId.current = null;
                setDetail(null);
                setDetailLoading(false);
            });
        }
        return () => {
            cancelled = true;
        };
    }, [isOpen, log.id]);

    const debugContent = detail?.debug_content;

    return (
        <div className="flex flex-col gap-4 h-full min-h-0">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 min-h-0 flex-1">
                <div className="flex flex-col rounded-2xl border border-border bg-muted/30 overflow-hidden min-h-0">
                    <div className="flex items-center gap-2 px-3 md:px-4 py-2.5 md:py-3 border-b border-border bg-muted/50 shrink-0">
                        <Send className="size-4 text-green-500" />
                        <span className="text-sm font-medium text-card-foreground">{t('requestContent')}</span>
                        <Badge variant="secondary" className="ml-auto text-xs">
                            {log.input_tokens.toLocaleString()}{log.cached_tokens > 0 ? ` (${t('cachedTokens')}: ${log.cached_tokens.toLocaleString()})` : ''} {t('tokens')}
                        </Badge>
                    </div>
                    <div className="flex-1 overflow-auto min-h-0">
                        <DeferredJsonContent content={detail?.request_content} fallbackText={detailLoading ? '' : t('noRequestContent')} loading={detailLoading} />
                    </div>
                </div>
                <div className="flex flex-col rounded-2xl border border-border bg-muted/30 overflow-hidden min-h-0">
                    <div className="flex items-center gap-2 px-3 md:px-4 py-2.5 md:py-3 border-b border-border bg-muted/50 shrink-0">
                        <MessageSquare className="size-4 text-purple-500" />
                        <span className="text-sm font-medium text-card-foreground">{t('responseContent')}</span>
                        <Badge variant="secondary" className="ml-auto text-xs">
                            {log.output_tokens.toLocaleString()} {t('tokens')}
                        </Badge>
                    </div>
                    <div className="flex-1 overflow-auto min-h-0">
                        <ResponseLogContent content={detail?.response_content} fallbackText={detailLoading ? '' : t('noResponseContent')} loading={detailLoading} />
                    </div>
                </div>
            </div>
            {debugContent && (
                <div className="flex flex-col shrink-0 rounded-2xl border border-border bg-muted/30 overflow-hidden max-h-[200px]">
                    <div
                        className="flex items-center gap-2 px-3 md:px-4 py-2.5 md:py-3 border-b border-border bg-muted/50 shrink-0 cursor-pointer select-none hover:bg-muted/70 transition-colors"
                        onClick={() => setDebugExpanded(!debugExpanded)}
                    >
                        <AlertCircle className="size-4 text-amber-500" />
                        <span className="text-sm font-medium text-card-foreground">调试信息</span>
                        <ChevronDown className={cn(
                            "size-4 ml-auto text-muted-foreground transition-transform duration-200",
                            debugExpanded && "rotate-180"
                        )} />
                    </div>
                    <AnimatePresence initial={false}>
                        {debugExpanded && (
                            <motion.div
                                initial={{ height: 0, opacity: 0 }}
                                animate={{ height: "auto", opacity: 1 }}
                                exit={{ height: 0, opacity: 0 }}
                                transition={{ duration: 0.2, ease: "easeInOut" }}
                                className="overflow-auto"
                            >
                                <DeferredJsonContent content={debugContent} fallbackText="" />
                            </motion.div>
                        )}
                    </AnimatePresence>
                </div>
            )}
        </div>
    );
}

export function LogCard({ log }: { log: RelayLog }) {
    const t = useTranslations('log.card');
    const { Avatar: ModelAvatar, color: brandColor } = useMemo(
        () => getModelIcon(log.actual_model_name),
        [log.actual_model_name]
    );
    const requestAPIKeyName = useMemo(() => log.request_api_key_name?.trim() ?? '', [log.request_api_key_name]);

    const hasError = !!log.error;
    const hasMultipleAttempts = log.attempts && log.attempts.length > 1;
    const [isDiagnosticExpanded, setIsDiagnosticExpanded] = useState(false);
    // 顶部 Badge 显示最终渠道，keyRemark 需匹配该渠道名
    const topKeyRemark = log.attempts?.find(a => a.channel_name === log.channel_name && a.status === 'success')?.channel_key_remark
        || log.attempts?.find(a => a.channel_name === log.channel_name)?.channel_key_remark;
    const channelDisplay = topKeyRemark ? `${log.channel_name}-${topKeyRemark}` : log.channel_name;

    return (
        <TooltipProvider>
            <MorphingDialog>
                <MorphingDialogTrigger
                    className={cn(
                        "rounded-3xl border bg-card w-full text-left",
                        hasError ? "border-destructive/40" : "border-border",
                    )}
                >
                    <div className={cn("p-4 grid grid-cols-[auto_1fr] gap-4", hasError ? "items-start" : "items-center")}>
                        <div className="relative shrink-0">
                            <ModelAvatar size={40} />
                            {log.client_name && (
                                <span className="absolute -bottom-0.5 -right-0.5">
                                    <ClientIconBadge clientName={log.client_name} className="size-4" />
                                </span>
                            )}
                        </div>
                        <div className="min-w-0 flex flex-col gap-3">
                            <div className="flex items-center gap-2 min-w-0 text-sm">
                                <span className="font-semibold text-card-foreground truncate" title={log.request_model_name}>
                                    {log.request_model_name}
                                </span>
                                <ArrowRight className="size-3.5 shrink-0 text-muted-foreground/50" />
                                {hasMultipleAttempts ? (
                                    <RetryBadgeWithTooltip
                                        channelName={channelDisplay}
                                        brandColor={brandColor}
                                        attempts={log.attempts!}
                                    />
                                ) : (
                                    <Badge
                                        variant="secondary"
                                        className="shrink-0 text-xs px-1.5 py-0"
                                        style={{ backgroundColor: `${brandColor}15`, color: brandColor }}
                                    >
                                        {channelDisplay}
                                    </Badge>
                                )}
                                <span className="text-muted-foreground truncate" title={log.actual_model_name}>
                                    {log.actual_model_name}
                                </span>
                                {log.reasoning_effort && (
                                    <ReasoningEffortBadge effort={log.reasoning_effort} />
                                )}
                                {log.attempts?.some(a => a.sticky) && (
                                    <Pin className="size-3.5 shrink-0 text-amber-500" />
                                )}
                            </div>
                            <div className="grid grid-cols-[repeat(auto-fill,110px)] gap-x-2 gap-y-1 text-xs tabular-nums text-muted-foreground">
                                <div className="flex items-center gap-1 overflow-hidden">
                                    <Clock className="size-3.5 shrink-0" style={{ color: brandColor }} />
                                    <span className="truncate">{formatTime(log.time)}</span>
                                </div>
                                {requestAPIKeyName && (
                                    <div className="flex items-center gap-1 overflow-hidden">
                                        <KeyRound className="size-3.5 shrink-0 text-orange-500" />
                                        <span className="truncate" title={requestAPIKeyName}>
                                            {requestAPIKeyName}
                                        </span>
                                    </div>
                                )}
                                <div className="flex items-center gap-1 overflow-hidden">
                                    <Zap className="size-3.5 shrink-0 text-amber-500" />
                                    <span className="truncate">{t('firstToken')} {formatDuration(log.ftut)}</span>
                                </div>
                                <div className="flex items-center gap-1 overflow-hidden">
                                    <Cpu className="size-3.5 shrink-0 text-blue-500" />
                                    <span className="truncate">{t('totalTime')} {formatDuration(log.use_time)}</span>
                                </div>
                                <div className="flex items-center gap-1 overflow-hidden">
                                    <ArrowDownToLine className="size-3.5 shrink-0 text-green-500" />
                                    <span className="truncate">{t('input')} {formatTokenCount(log.input_tokens)}</span>
                                </div>
                                {log.cached_tokens > 0 && (
                                    <div className="flex items-center gap-1 overflow-hidden">
                                        <Database className="size-3.5 shrink-0 text-orange-500" />
                                        <span className="truncate">{t('cachedTokens')} {formatTokenCount(log.cached_tokens)}</span>
                                    </div>
                                )}
                                <div className="flex items-center gap-1 overflow-hidden">
                                    <ArrowUpFromLine className="size-3.5 shrink-0 text-purple-500" />
                                    <span className="truncate">{t('output')} {formatTokenCount(log.output_tokens)}</span>
                                </div>
                                {(() => {
                                    const outputTime = log.use_time - log.ftut;
                                    const tps = outputTime > 0 ? (log.output_tokens / outputTime * 1000) : (log.use_time > 0 ? (log.output_tokens / log.use_time * 1000) : 0);
                                    return tps > 0 ? (
                                        <div className="flex items-center gap-1 overflow-hidden">
                                            <Gauge className="size-3.5 shrink-0 text-cyan-500" />
                                            <span className="truncate">{t('tps')} {tps.toFixed(1)}</span>
                                        </div>
                                    ) : null;
                                })()}
                                <div className="flex items-center gap-1 overflow-hidden">
                                    <DollarSign className="size-3.5 shrink-0 text-emerald-500" />
                                    <span className="truncate font-medium text-emerald-600 dark:text-emerald-400">
                                        {t('cost')} {Number(log.cost).toFixed(6)}
                                    </span>
                                </div>
                            </div>
                            {hasError && (
                                <div className="p-2.5 rounded-xl bg-destructive/10 border border-destructive/20 overflow-hidden">
                                    <p className="text-xs text-destructive line-clamp-2">{log.error}</p>
                                </div>
                            )}
                        </div>
                    </div>
                </MorphingDialogTrigger>

                <MorphingDialogContainer>
                    <MorphingDialogContent className="relative w-[calc(100vw-2rem)] md:w-[80vw] bg-card text-card-foreground px-6 py-4 rounded-3xl h-[calc(100vh-2rem)] flex flex-col overflow-hidden">
                        <MorphingDialogClose className="top-4 right-5 text-muted-foreground hover:text-foreground transition-colors" />
                        <MorphingDialogTitle className="flex items-center gap-2 mb-3 text-sm">
                            <div className="relative shrink-0">
                                <ModelAvatar size={28} />
                                {log.client_name && (
                                    <ClientIconBadge clientName={log.client_name} className="size-3.5" />
                                )}
                            </div>
                            <span className="font-semibold text-card-foreground">{log.request_model_name}</span>
                            <ArrowRight className="size-3.5 text-muted-foreground/50" />
                            {hasMultipleAttempts ? (
                                <RetryBadgeWithTooltip
                                    channelName={channelDisplay}
                                    brandColor={brandColor}
                                    attempts={log.attempts!}
                                />
                            ) : (
                                <Badge
                                    variant="secondary"
                                    className="text-xs px-1.5 py-0"
                                    style={{ backgroundColor: `${brandColor}15`, color: brandColor }}
                                >
                                    {channelDisplay}
                                </Badge>
                            )}
                            <span className="text-muted-foreground">{log.actual_model_name}</span>
                            {log.reasoning_effort && (
                                <ReasoningEffortBadge effort={log.reasoning_effort} />
                            )}
                            {log.attempts?.some(a => a.sticky) && (
                                <Pin className="size-3.5 shrink-0 text-amber-500" />
                            )}
                        </MorphingDialogTitle>

                        <MorphingDialogDescription className="flex-1 min-h-0">
                            <div className="flex flex-col min-h-0 h-full gap-4">
                                {(hasError || hasMultipleAttempts) && (
                                    <div className={cn(
                                        "flex-initial min-h-0 flex flex-col rounded-2xl border overflow-hidden max-h-[40%]",
                                        hasError
                                            ? "bg-destructive/5 border-destructive/20"
                                            : "bg-secondary/30 border-border/50"
                                    )}>
                                        <div
                                            className={cn(
                                                "flex items-center gap-2 px-3 py-2.5 shrink-0 cursor-pointer select-none hover:bg-muted/50 transition-colors",
                                                hasError && "hover:bg-destructive/10"
                                            )}
                                            onClick={() => setIsDiagnosticExpanded(!isDiagnosticExpanded)}
                                        >
                                            {hasError ? (
                                                <AlertCircle className="size-4 text-destructive" />
                                            ) : (
                                                <RotateCw className="size-4 text-muted-foreground" />
                                            )}
                                            <span className={cn(
                                                "text-sm font-medium",
                                                hasError ? "text-destructive" : "text-secondary-foreground"
                                            )}>
                                                {hasError ? t('errorInfo') : t('retryDetails')}
                                            </span>
                                            <div className="ml-auto flex items-center gap-2">
                                                {hasMultipleAttempts && (
                                                    <Badge
                                                        variant="outline"
                                                        className={cn(
                                                            "text-xs border-0",
                                                            hasError
                                                                ? "bg-destructive/10 text-destructive"
                                                                : "bg-secondary text-secondary-foreground"
                                                        )}
                                                    >
                                                        {log.total_attempts || log.attempts!.length} {t('attempts')}
                                                    </Badge>
                                                )}
                                                {isDiagnosticExpanded ? (
                                                    <ChevronUp className="size-4 text-muted-foreground" />
                                                ) : (
                                                    <ChevronDown className="size-4 text-muted-foreground" />
                                                )}
                                            </div>
                                        </div>

                                        <AnimatePresence initial={false}>
                                            {isDiagnosticExpanded && (
                                                <motion.div
                                                    initial={{ height: 0, opacity: 0 }}
                                                    animate={{ height: "auto", opacity: 1 }}
                                                    exit={{ height: 0, opacity: 0 }}
                                                    transition={{ duration: 0.2, ease: "easeInOut" }}
                                                    className="overflow-hidden flex flex-col min-h-0"
                                                >
                                                    <div className="flex-1 overflow-auto p-2.5 md:p-3 flex flex-col gap-4">
                                                        {hasError && (
                                                            <div className="relative pl-1">
                                                                <div className="absolute right-0 top-0">
                                                                    <CopyIconButton
                                                                        text={log.error ?? ''}
                                                                        className="p-1 rounded-md text-destructive/60 hover:text-destructive hover:bg-destructive/10 transition-colors"
                                                                        copyIconClassName="size-4"
                                                                        checkIconClassName="size-4"
                                                                    />
                                                                </div>
                                                                <p className="text-sm text-destructive whitespace-pre-wrap wrap-break-word pr-8 leading-relaxed">
                                                                    {log.error}
                                                                </p>
                                                            </div>
                                                        )}

                                                        {hasMultipleAttempts && (
                                                            <div className="flex flex-col gap-2">
                                                                {log.attempts!.map((attempt, idx) => (
                                                                    <div
                                                                        key={idx}
                                                                        className={cn(
                                                                            "text-xs p-2.5 rounded-xl border transition-colors flex flex-col gap-2",
                                                                            attempt.status === 'success'
                                                                                ? "bg-primary/5 border-primary/20 hover:bg-primary/10"
                                                                                : "bg-destructive/5 border-destructive/20 hover:bg-destructive/10"
                                                                        )}
                                                                    >
                                                                        <div className="flex items-center gap-2">
                                                                            <span className="font-semibold text-foreground">
                                                                                {attempt.channel_key_remark ? `${attempt.channel_name}-${attempt.channel_key_remark}` : attempt.channel_name}
                                                                            </span>
                                                                            <span className="text-muted-foreground">
                                                                                ({attempt.model_name})
                                                                            </span>
                                                                            <span className="ml-auto text-muted-foreground tabular-nums font-mono">
                                                                                {formatDuration(attempt.duration)}
                                                                            </span>
                                                                        </div>
                                                                        {attempt.msg && (
                                                                            <div className="text-destructive/90 pl-2 border-l-2 border-destructive/30 text-[11px] leading-relaxed">
                                                                                {attempt.msg}
                                                                            </div>
                                                                        )}
                                                                    </div>
                                                                ))}
                                                            </div>
                                                        )}
                                                    </div>
                                                </motion.div>
                                            )}
                                        </AnimatePresence>
                                    </div>
                                )}
                                <div className="flex-1 min-h-0 overflow-hidden">
                                    <LogDetailPanels log={log} />
                                </div>
                            </div>
                        </MorphingDialogDescription>

                        <div className="grid grid-cols-[repeat(auto-fill,140px)] gap-x-3 gap-y-1 pt-4 mt-auto text-xs tabular-nums text-muted-foreground shrink-0">
                            <div className="flex items-center gap-1 overflow-hidden">
                                <Clock className="size-3.5 shrink-0" style={{ color: brandColor }} />
                                <span className="truncate">{formatTime(log.time)}</span>
                            </div>
                            {requestAPIKeyName && (
                                <div className="flex items-center gap-1 overflow-hidden">
                                    <KeyRound className="size-3.5 shrink-0 text-orange-500" />
                                    <span className="truncate" title={requestAPIKeyName}>
                                        {requestAPIKeyName}
                                    </span>
                                </div>
                            )}
                            <div className="flex items-center gap-1 overflow-hidden">
                                <Zap className="size-3.5 shrink-0 text-amber-500" />
                                <span className="truncate">{t('firstTokenTime')}: {formatDuration(log.ftut)}</span>
                            </div>
                            <div className="flex items-center gap-1 overflow-hidden">
                                <Cpu className="size-3.5 shrink-0 text-blue-500" />
                                <span className="truncate">{t('totalTime')}: {formatDuration(log.use_time)}</span>
                            </div>
                            {(() => {
                                const outputTime = log.use_time - log.ftut;
                                const tps = outputTime > 0 ? (log.output_tokens / outputTime * 1000) : (log.use_time > 0 ? (log.output_tokens / log.use_time * 1000) : 0);
                                return tps > 0 ? (
                                    <div className="flex items-center gap-1 overflow-hidden">
                                        <Gauge className="size-3.5 shrink-0 text-cyan-500" />
                                        <span className="truncate">{t('tps')}: {tps.toFixed(1)} tok/s</span>
                                    </div>
                                ) : null;
                            })()}
                            <div className="flex items-center gap-1 overflow-hidden">
                                <DollarSign className="size-3.5 shrink-0 text-emerald-500" />
                                <span className="truncate font-medium text-emerald-600 dark:text-emerald-400">
                                    {t('cost')}: {Number(log.cost).toFixed(6)}
                                </span>
                            </div>
                        </div>
                    </MorphingDialogContent>
                </MorphingDialogContainer>
            </MorphingDialog>
        </TooltipProvider>
    );
}
