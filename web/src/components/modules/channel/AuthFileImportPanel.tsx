'use client';

import { useCodexAuthImport, type CodexAuthImportBatchResult } from '@/api/endpoints/channel';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { toast } from '@/components/common/Toast';
import { useTranslations } from 'next-intl';
import { useState, useRef, useCallback, useMemo } from 'react';
import { Upload, Loader2, CheckCircle2, XCircle, AlertTriangle, FileJson, FileArchive } from 'lucide-react';

const MAX_FILES = 200; // 单次导入最大文件数

interface AuthFileImportPanelProps {
  channelId: number;
  onImportComplete: () => void;
}

interface PreviewItem {
  file: File;
  source: 'json' | 'zip';
  email?: string;
  account_id?: string;
  valid: boolean;
  error?: string;
}

function parsePreviewInfo(file: File, data: string): { email?: string; account_id?: string; valid: boolean; error?: string } {
  try {
    const parsed = JSON.parse(data);
    if (parsed.type !== 'codex') {
      return { valid: false, error: 'type is not codex' };
    }
    if (!parsed.refresh_token) {
      return { valid: false, error: 'missing refresh_token' };
    }
    return {
      email: parsed.email || undefined,
      account_id: parsed.account_id || undefined,
      valid: true,
    };
  } catch {
    return { valid: false, error: 'invalid JSON' };
  }
}

async function buildPreviewItems(files: File[]): Promise<PreviewItem[]> {
  // 并行读取所有 JSON 文件，ZIP 文件不需要读取内容
  const results = await Promise.all(files.map(async (file) => {
    const lowerName = file.name.toLowerCase();
    if (lowerName.endsWith('.zip')) {
      return { file, source: 'zip' as const, valid: true } as PreviewItem;
    } else if (lowerName.endsWith('.json')) {
      try {
        const text = await file.text();
        const info = parsePreviewInfo(file, text);
        return {
          file,
          source: 'json' as const,
          email: info.email,
          account_id: info.account_id,
          valid: info.valid,
          error: info.error,
        } as PreviewItem;
      } catch {
        return { file, source: 'json' as const, valid: false, error: 'failed to read file' } as PreviewItem;
      }
    } else {
      return { file, source: 'json' as const, valid: false, error: 'not a json or zip file' } as PreviewItem;
    }
  }));
  return results;
}

function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'imported':
    case 'updated':
      return <CheckCircle2 className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />;
    case 'incomplete':
      return <AlertTriangle className="h-3.5 w-3.5 text-amber-600 dark:text-amber-400" />;
    case 'failed':
      return <XCircle className="h-3.5 w-3.5 text-red-600 dark:text-red-400" />;
    case 'skipped':
    case 'duplicate_in_archive':
      return <AlertTriangle className="h-3.5 w-3.5 text-muted-foreground" />;
    default:
      return null;
  }
}

// 虚拟滚动行渲染：只渲染可视区域内的行
function VirtualizedTable<T>({ items, rowHeight, maxHeight, renderRow }: {
  items: T[];
  rowHeight: number;
  maxHeight: number;
  renderRow: (item: T, idx: number, style: { position: 'absolute'; top: number; height: number; left: number; right: number }) => React.ReactNode;
}) {
  const [scrollTop, setScrollTop] = useState(0);
  const totalHeight = items.length * rowHeight;
  const visibleCount = Math.ceil(maxHeight / rowHeight) + 2;
  const startIdx = Math.max(0, Math.floor(scrollTop / rowHeight) - 1);
  const endIdx = Math.min(items.length, startIdx + visibleCount);

  return (
    <div
      className="overflow-y-auto"
      style={{ maxHeight }}
      onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
    >
      <div style={{ height: totalHeight, position: 'relative' }}>
        {items.slice(startIdx, endIdx).map((item, i) => {
          const idx = startIdx + i;
          return renderRow(item, idx, {
            position: 'absolute',
            top: idx * rowHeight,
            height: rowHeight,
            left: 0,
            right: 0,
          });
        })}
      </div>
    </div>
  );
}

export function AuthFileImportPanel({ channelId, onImportComplete }: AuthFileImportPanelProps) {
  const t = useTranslations('channel.form');
  const importMutation = useCodexAuthImport();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [previewItems, setPreviewItems] = useState<PreviewItem[]>([]);
  const [selectedIndices, setSelectedIndices] = useState<Set<number>>(new Set());
  const [importResult, setImportResult] = useState<CodexAuthImportBatchResult | null>(null);
  const [step, setStep] = useState<'select' | 'preview' | 'result'>('select');

  // 统计摘要
  const summary = useMemo(() => {
    const valid = previewItems.filter(i => i.valid).length;
    const invalid = previewItems.length - valid;
    return { valid, invalid, total: previewItems.length };
  }, [previewItems]);

  const selectedCount = selectedIndices.size;

  const handleFileSelect = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files || files.length === 0) return;

    const fileArray = Array.from(files);
    if (fileArray.length > MAX_FILES) {
      toast.error(t('importAuthFilesFailed'), {
        description: t('importTooManyFiles', { max: MAX_FILES, count: fileArray.length }),
      });
      if (fileInputRef.current) fileInputRef.current.value = '';
      return;
    }

    const items = await buildPreviewItems(fileArray);
    setPreviewItems(items);
    const initialSelected = new Set<number>();
    items.forEach((item, idx) => {
      if (item.valid) initialSelected.add(idx);
    });
    setSelectedIndices(initialSelected);
    setImportResult(null);
    setStep('preview');

    if (fileInputRef.current) fileInputRef.current.value = '';
  }, [t]);

  const handleImport = useCallback(async () => {
    const files = previewItems
      .filter((_, idx) => selectedIndices.has(idx))
      .map(item => item.file);
    if (files.length === 0) return;

    try {
      const result = await importMutation.mutateAsync({ channelId, files });
      setImportResult(result);
      setStep('result');
      if (result.imported > 0 || result.updated > 0) {
        onImportComplete();
      }
    } catch (error) {
      toast.error(t('importAuthFilesFailed'), {
        description: error instanceof Error ? error.message : String(error),
      });
    }
  }, [previewItems, selectedIndices, channelId, importMutation, onImportComplete, t]);

  const handleReset = useCallback(() => {
    setPreviewItems([]);
    setSelectedIndices(new Set());
    setImportResult(null);
    setStep('select');
  }, []);

  const toggleItem = (idx: number) => {
    setSelectedIndices(prev => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx); else next.add(idx);
      return next;
    });
  };

  const toggleAll = () => {
    const validIndices = new Set(previewItems.map((item, i) => item.valid ? i : -1));
    // 如果所有有效项都已选中，则全不选；否则选中所有有效项
    const allValidSelected = previewItems.every((item, i) => !item.valid || selectedIndices.has(i));
    if (allValidSelected) {
      setSelectedIndices(new Set());
    } else {
      setSelectedIndices(new Set(previewItems.map((_, i) => i).filter(i => previewItems[i].valid)));
    }
  };

  // 选择文件按钮
  if (step === 'select') {
    return (
      <>
        <input
          ref={fileInputRef}
          type="file"
          accept=".json,.zip"
          multiple
          onChange={handleFileSelect}
          className="hidden"
        />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => fileInputRef.current?.click()}
          className="h-6 px-2 text-xs text-muted-foreground/70 hover:text-muted-foreground hover:bg-transparent"
        >
          <Upload className="h-3 w-3 mr-1" />
          {t('importAuthFiles')}
        </Button>
        <p className="text-xs text-muted-foreground mt-1">{t('importAuthFilesHint')}</p>
      </>
    );
  }

  // 预览表格（带勾选 + 虚拟滚动）
  if (step === 'preview') {
    const ROW_HEIGHT = 28;
    const TABLE_MAX_H = 200;

    return (
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-card-foreground">
            {t('importPreview')} ({selectedCount}/{summary.total})
          </span>
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={handleImport}
              disabled={importMutation.isPending || selectedCount === 0}
              className="h-6 px-2 text-xs"
            >
              {importMutation.isPending ? <Loader2 className="h-3 w-3 mr-1 animate-spin" /> : <Upload className="h-3 w-3 mr-1" />}
              {t('importConfirm')}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={handleReset}
              className="h-6 px-2 text-xs text-muted-foreground"
            >
              {t('importCancel')}
            </Button>
          </div>
        </div>
        {/* 摘要栏 */}
        <div className="flex items-center gap-2 text-xs">
          <Badge variant="outline" className="text-[10px] px-1.5 py-0 text-green-700 dark:text-green-400 border-green-300 dark:border-green-700">
            {t('importStatusReady')}: {summary.valid}
          </Badge>
          {summary.invalid > 0 && (
            <Badge variant="outline" className="text-[10px] px-1.5 py-0 text-red-700 dark:text-red-400 border-red-300 dark:border-red-700">
              {t('failed')}: {summary.invalid}
            </Badge>
          )}
          <Checkbox
            checked={selectedCount === summary.total && summary.total > 0}
            onCheckedChange={toggleAll}
            className="scale-75"
          />
          <span className="text-muted-foreground">{t('importSelectAll')}</span>
        </div>
        {/* 虚拟滚动表格 */}
        <div className="rounded-xl border border-border bg-muted/30">
          <div className="border-b border-border flex items-center text-xs font-medium text-muted-foreground px-2" style={{ height: ROW_HEIGHT }}>
            <span className="w-6 shrink-0"></span>
            <span className="flex-1 min-w-0">{t('importColFile')}</span>
            <span className="w-12 shrink-0">{t('importColSource')}</span>
            <span className="w-28 shrink-0 truncate">{t('importColAccount')}</span>
            <span className="w-20 shrink-0">{t('importColStatus')}</span>
          </div>
          <VirtualizedTable
            items={previewItems}
            rowHeight={ROW_HEIGHT}
            maxHeight={TABLE_MAX_H}
            renderRow={(item, idx, style) => (
              <div
                key={idx}
                style={style}
                className={`flex items-center text-xs px-2 border-b border-border/30 ${!selectedIndices.has(idx) ? 'opacity-40' : ''}`}
              >
                <span className="w-6 shrink-0">
                  <Checkbox
                    checked={selectedIndices.has(idx)}
                    onCheckedChange={() => toggleItem(idx)}
                    className="scale-75"
                  />
                </span>
                <span className="flex-1 min-w-0 flex items-center gap-1">
                  {item.source === 'zip' ? <FileArchive className="h-3 w-3 shrink-0" /> : <FileJson className="h-3 w-3 shrink-0" />}
                  <span className="truncate">{item.file.name}</span>
                </span>
                <span className="w-12 shrink-0 uppercase">{item.source}</span>
                <span className="w-28 shrink-0 truncate">{item.email || item.account_id || '-'}</span>
                <span className="w-20 shrink-0">
                  {item.valid ? (
                    <Badge variant="outline" className="text-[10px] px-1 py-0 text-green-700 dark:text-green-400 border-green-300 dark:border-green-700">
                      OK
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="text-[10px] px-1 py-0 text-red-700 dark:text-red-400 border-red-300 dark:border-red-700">
                      {item.error}
                    </Badge>
                  )}
                </span>
              </div>
            )}
          />
        </div>
      </div>
    );
  }

  // 导入结果（虚拟滚动）
  if (step === 'result' && importResult) {
    const ROW_HEIGHT = 28;
    const TABLE_MAX_H = 200;

    return (
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-card-foreground">{t('importResult')}</span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={handleReset}
            className="h-6 px-2 text-xs text-muted-foreground"
          >
            {t('importDone')}
          </Button>
        </div>
        <div className="flex items-center gap-3 text-xs">
          <span className="text-green-700 dark:text-green-400">{t('imported')}: {importResult.imported}</span>
          <span className="text-blue-700 dark:text-blue-400">{t('updated')}: {importResult.updated}</span>
          <span className="text-red-700 dark:text-red-400">{t('failed')}: {importResult.failed}</span>
          <span className="text-muted-foreground">{t('skipped')}: {importResult.skipped}</span>
        </div>
        {importResult.results.length > 0 && (
          <div className="rounded-xl border border-border bg-muted/30">
            <div className="border-b border-border flex items-center text-xs font-medium text-muted-foreground px-2" style={{ height: ROW_HEIGHT }}>
              <span className="flex-1 min-w-0">{t('importColFile')}</span>
              <span className="w-12 shrink-0">{t('importColSource')}</span>
              <span className="w-28 shrink-0 truncate">{t('importColAccount')}</span>
              <span className="w-24 shrink-0">{t('importColStatus')}</span>
            </div>
            <VirtualizedTable
              items={importResult.results}
              rowHeight={ROW_HEIGHT}
              maxHeight={TABLE_MAX_H}
              renderRow={(r, idx, style) => (
                <div
                  key={idx}
                  style={style}
                  className="flex items-center text-xs px-2 border-b border-border/30"
                >
                  <span className="flex-1 min-w-0 truncate">{r.file}</span>
                  <span className="w-12 shrink-0 uppercase">{r.source}</span>
                  <span className="w-28 shrink-0 truncate">{r.email || r.account_id || '-'}</span>
                  <span className="w-24 shrink-0 flex items-center gap-1">
                    <StatusIcon status={r.status} />
                    <span className="truncate">{r.status}{r.error ? `: ${r.error}` : ''}</span>
                  </span>
                </div>
              )}
            />
          </div>
        )}
      </div>
    );
  }

  return null;
}
