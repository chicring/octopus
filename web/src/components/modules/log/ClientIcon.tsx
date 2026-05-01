'use client';

import { useTranslations } from 'next-intl';
import {
    Terminal, Code2, Bot, MonitorSmartphone, Sparkles, Cpu,
    Wrench, Zap, Globe, FileCode, GitBranch, Blocks, Rocket,
    Feather, Bird, Cog, Compass, Fingerprint, Workflow,
    Triangle, Diamond, Star, Moon, Cloud,
    Braces, Binary, Scan, PenTool, Puzzle, Squirrel, Fish,
    type LucideIcon,
} from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';

type ClientConfig = {
    icon: LucideIcon;
    color: string;
    labelKey: string;
};

const CLIENT_CONFIGS: Record<string, ClientConfig> = {
    'claude-code': { icon: Terminal, color: '#D7765A', labelKey: 'claudeCode' },
    'cline': { icon: Code2, color: '#6B4EFF', labelKey: 'cline' },
    'roo-code': { icon: Bot, color: '#8B5CF6', labelKey: 'rooCode' },
    'cursor': { icon: MonitorSmartphone, color: '#6366F1', labelKey: 'cursor' },
    'windsurf': { icon: Sparkles, color: '#09B6A2', labelKey: 'windsurf' },
    'copilot': { icon: Cpu, color: '#000000', labelKey: 'copilot' },
    'aider': { icon: Terminal, color: '#FF6B00', labelKey: 'aider' },
    'codex': { icon: Terminal, color: '#10A37F', labelKey: 'codex' },
    'continue': { icon: Wrench, color: '#6366F1', labelKey: 'continue' },
    'amazon-q': { icon: Zap, color: '#FF9900', labelKey: 'amazonQ' },
    'augment': { icon: Globe, color: '#7C3AED', labelKey: 'augment' },
    'amp': { icon: Zap, color: '#F59E0B', labelKey: 'amp' },
    'auto-coder': { icon: FileCode, color: '#3B82F6', labelKey: 'autoCoder' },
    'codebuddy': { icon: Bot, color: '#22C55E', labelKey: 'codebuddy' },
    'codebuff': { icon: GitBranch, color: '#F97316', labelKey: 'codebuff' },
    'codegpt': { icon: Bot, color: '#10A37F', labelKey: 'codegpt' },
    'crush': { icon: Diamond, color: '#EF4444', labelKey: 'crush' },
    'factory-droid': { icon: Cog, color: '#6366F1', labelKey: 'factoryDroid' },
    'gemini-cli': { icon: Terminal, color: '#4285F4', labelKey: 'geminiCli' },
    'gemini-code-assist': { icon: Compass, color: '#4285F4', labelKey: 'geminiCodeAssist' },
    'goose': { icon: Bird, color: '#F59E0B', labelKey: 'goose' },
    'jules': { icon: Feather, color: '#4285F4', labelKey: 'jules' },
    'junie': { icon: Star, color: '#8B5CF6', labelKey: 'junie' },
    'kilo-code': { icon: Blocks, color: '#22C55E', labelKey: 'kiloCode' },
    'kiro': { icon: Fingerprint, color: '#3B82F6', labelKey: 'kiro' },
    'opencode': { icon: Braces, color: '#6366F1', labelKey: 'opencode' },
    'openhands': { icon: Workflow, color: '#F97316', labelKey: 'openhands' },
    'qoder': { icon: Binary, color: '#8B5CF6', labelKey: 'qoder' },
    'qwen-code': { icon: Terminal, color: '#6B4EFF', labelKey: 'qwenCode' },
    'replit': { icon: Rocket, color: '#F26207', labelKey: 'replit' },
    'rovidev': { icon: Scan, color: '#3B82F6', labelKey: 'rovidev' },
    'tabnine': { icon: Puzzle, color: '#6B4EFF', labelKey: 'tabnine' },
    'trae': { icon: PenTool, color: '#10B981', labelKey: 'trae' },
    'warp': { icon: Cloud, color: '#01A4FF', labelKey: 'warp' },
    'zed': { icon: Squirrel, color: '#E8E8E8', labelKey: 'zed' },
    'baidu-comate': { icon: Fish, color: '#2932E1', labelKey: 'baiduComate' },
    'tongyi-lingma': { icon: Moon, color: '#6B4EFF', labelKey: 'tongyiLingma' },
    'anthropic-ts': { icon: Triangle, color: '#D7765A', labelKey: 'anthropicTs' },
    'openai-python': { icon: Triangle, color: '#10A37F', labelKey: 'openaiPython' },
    'openai-js': { icon: Triangle, color: '#10A37F', labelKey: 'openaiJs' },
};

interface ClientIconBadgeProps {
    clientName: string;
    className?: string;
}

export function ClientIconBadge({ clientName, className }: ClientIconBadgeProps) {
    const t = useTranslations('log.client');

    const config = CLIENT_CONFIGS[clientName];
    if (!config) return null;

    const Icon = config.icon;

    return (
        <Tooltip>
            <TooltipTrigger asChild>
                <span
                    className={`inline-flex items-center justify-center shrink-0 ${className ?? 'size-4'}`}
                    style={{ color: config.color }}
                >
                    <Icon className="size-full" />
                </span>
            </TooltipTrigger>
            <TooltipContent className="border bg-card px-2 py-1 text-xs shadow-sm rounded-xl">
                {t(config.labelKey)}
            </TooltipContent>
        </Tooltip>
    );
}
