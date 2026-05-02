'use client';

import Image from 'next/image';
import { useTranslations } from 'next-intl';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';

type ClientConfig = {
    icon: string;   // /clients/{name}.png
    labelKey: string; // i18n key in log.client
};

const CLIENT_CONFIGS: Record<string, ClientConfig> = {
    'claude-code':          { icon: '/clients/claude-code.png',          labelKey: 'claudeCode' },
    'cline':                { icon: '/clients/cline.png',                labelKey: 'cline' },
    'roo-code':             { icon: '/clients/roo-code.png',             labelKey: 'rooCode' },
    'cursor':               { icon: '/clients/cursor.png',               labelKey: 'cursor' },
    'windsurf':             { icon: '/clients/windsurf.png',             labelKey: 'windsurf' },
    'copilot':              { icon: '/clients/copilot.png',              labelKey: 'copilot' },
    'aider':                { icon: '/clients/aider.png',                labelKey: 'aider' },
    'codex':                { icon: '/clients/codex.png',                labelKey: 'codex' },
    'continue':             { icon: '/clients/continue.png',             labelKey: 'continue' },
    'amazon-q':             { icon: '/clients/amazon-q.png',             labelKey: 'amazonQ' },
    'augment':              { icon: '/clients/augment.png',              labelKey: 'augment' },
    'amp':                  { icon: '/clients/amp.png',                  labelKey: 'amp' },
    'auto-coder':           { icon: '/clients/auto-coder.png',           labelKey: 'autoCoder' },
    'codebuddy':            { icon: '/clients/kilo-code.png',            labelKey: 'codebuddy' },
    'codebuff':             { icon: '/clients/kilo-code.png',            labelKey: 'codebuff' },
    'codegpt':              { icon: '/clients/kilo-code.png',            labelKey: 'codegpt' },
    'crush':                { icon: '/clients/crush.png',                labelKey: 'crush' },
    'factory-droid':        { icon: '/clients/factory-droid.png',        labelKey: 'factoryDroid' },
    'gemini-cli':           { icon: '/clients/gemini-cli.png',           labelKey: 'geminiCli' },
    'gemini-code-assist':   { icon: '/clients/gemini-code-assist.png',   labelKey: 'geminiCodeAssist' },
    'goose':                { icon: '/clients/goose.png',                labelKey: 'goose' },
    'jules':                { icon: '/clients/jules.png',                labelKey: 'jules' },
    'junie':                { icon: '/clients/junie.png',                labelKey: 'junie' },
    'kilo-code':            { icon: '/clients/kilo-code.png',            labelKey: 'kiloCode' },
    'kiro':                 { icon: '/clients/kiro.png',                 labelKey: 'kiro' },
    'opencode':             { icon: '/clients/opencode.png',             labelKey: 'opencode' },
    'openhands':            { icon: '/clients/openhands.png',            labelKey: 'openhands' },
    'qoder':                { icon: '/clients/qoder.png',                labelKey: 'qoder' },
    'qwen-code':            { icon: '/clients/tongyi-lingma.png',        labelKey: 'qwenCode' },
    'replit':               { icon: '/clients/replit.png',               labelKey: 'replit' },
    'rovidev':              { icon: '/clients/kilo-code.png',            labelKey: 'rovidev' },
    'tabnine':              { icon: '/clients/tabnine.png',              labelKey: 'tabnine' },
    'trae':                 { icon: '/clients/trae.png',                 labelKey: 'trae' },
    'warp':                 { icon: '/clients/warp.png',                 labelKey: 'warp' },
    'zed':                  { icon: '/clients/zed.png',                  labelKey: 'zed' },
    'baidu-comate':         { icon: '/clients/baidu-comate.png',         labelKey: 'baiduComate' },
    'tongyi-lingma':        { icon: '/clients/tongyi-lingma.png',        labelKey: 'tongyiLingma' },
    'anthropic-ts':         { icon: '/clients/anthropic-ts.png',         labelKey: 'anthropicTs' },
    'openai-python':        { icon: '/clients/openai-python.png',        labelKey: 'openaiPython' },
    'openai-js':            { icon: '/clients/openai-js.png',            labelKey: 'openaiJs' },
};

interface ClientIconBadgeProps {
    clientName: string;
    className?: string;
}

export function ClientIconBadge({ clientName, className }: ClientIconBadgeProps) {
    const t = useTranslations('log.client');

    const config = CLIENT_CONFIGS[clientName];
    if (!config) return null;

    return (
        <Tooltip>
            <TooltipTrigger asChild>
                <span className={`inline-flex items-center justify-center shrink-0 rounded-full bg-white dark:bg-muted/80 ${className ?? 'size-4'}`}>
                    <Image
                        src={config.icon}
                        alt={clientName}
                        width={16}
                        height={16}
                        className="size-[60%] object-contain"
                        unoptimized
                    />
                </span>
            </TooltipTrigger>
            <TooltipContent className="border bg-card px-2 py-1 text-xs shadow-sm rounded-xl">
                {t(config.labelKey)}
            </TooltipContent>
        </Tooltip>
    );
}
