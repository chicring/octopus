'use client';

import { useStartAuth, usePollAuth, useSubmitCallback, type AuthResult } from '@/api/endpoints/provider';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from '@/components/common/Toast';
import { Plus, Loader2 } from 'lucide-react';

interface OAuthPanelProps {
  providerId: string;
  authType: 'oauth_device' | 'oauth_web';
  onSuccess: (result: NonNullable<AuthResult['result']>) => void;
}

export function OAuthPanel({ providerId, authType, onSuccess }: OAuthPanelProps) {
  const t = useTranslations('provider.form');
  const startAuth = useStartAuth();
  const pollAuth = usePollAuth();
  const submitCallback = useSubmitCallback();
  const [sessionId, setSessionId] = useState<string>('');
  const [userCode, setUserCode] = useState<string>('');
  const [verificationUri, setVerificationUri] = useState<string>('');
  const [callbackUrl, setCallbackUrl] = useState<string>('');
  const [polling, setPolling] = useState(false);
  const [step, setStep] = useState<'idle' | 'waiting_callback'>('idle');

  const handleStart = useCallback(async () => {
    const result = await startAuth.mutateAsync({ provider_id: providerId });
    setSessionId(result.session_id);
    if (authType === 'oauth_device') {
      setUserCode(result.user_code || '');
      setVerificationUri(result.verification_uri || '');
      setPolling(true);
    } else {
      // oauth_web: 手动回调模式，显示授权链接等待用户粘贴回调 URL
      setVerificationUri(result.verification_uri || '');
      setStep('waiting_callback');
    }
  }, [providerId, authType, startAuth]);

  // 设备码轮询逻辑（仅 oauth_device）
  useEffect(() => {
    if (!polling || !sessionId || authType !== 'oauth_device') return;

    const interval = setInterval(async () => {
      try {
        const result = await pollAuth.mutateAsync({ session_id: sessionId });
        if (result.status === 'completed' && result.result?.access_token) {
          setPolling(false);
          onSuccess(result.result);
          toast.success(t('authSuccess'));
        } else if (result.status === 'failed') {
          setPolling(false);
          toast.error(t('authFailed'));
        }
      } catch {
        setPolling(false);
        toast.error(t('authFailed'));
      }
    }, 5000);

    return () => clearInterval(interval);
  }, [polling, sessionId, authType, pollAuth, onSuccess, t]);

  // 手动回调提交
  const handleSubmitCallback = useCallback(async () => {
    if (!callbackUrl.trim()) {
      toast.error(t('callbackUrlRequired'));
      return;
    }
    try {
      const result = await submitCallback.mutateAsync({
        session_id: sessionId,
        callback_url: callbackUrl.trim(),
      });
      if (result?.access_token) {
        setStep('idle');
        onSuccess(result);
        toast.success(t('authSuccess'));
      } else {
        toast.error(t('authFailed'));
      }
    } catch {
      toast.error(t('authFailed'));
    }
  }, [callbackUrl, sessionId, submitCallback, onSuccess, t]);

  // oauth_device 轮询中
  if (polling && authType === 'oauth_device') {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('authDeviceInstruction')}</p>
        <div className="flex items-center gap-2">
          <Input value={userCode} readOnly className="font-mono" />
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              navigator.clipboard.writeText(userCode);
              toast.success(t('copied'));
            }}
          >
            {t('copy')}
          </Button>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={() => window.open(verificationUri, '_blank')}>
          {t('openVerification')}
        </Button>
        <p className="text-xs text-muted-foreground">{t('authPolling')}</p>
      </div>
    );
  }

  // oauth_web 手动回调模式：等待用户粘贴回调 URL
  if (step === 'waiting_callback' && authType === 'oauth_web') {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('authWebManualStep1')}</p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => window.open(verificationUri, '_blank')}
        >
          {t('openAuthPage')}
        </Button>
        <p className="text-sm text-muted-foreground">{t('authWebManualStep2')}</p>
        <Input
          value={callbackUrl}
          onChange={(e) => setCallbackUrl(e.target.value)}
          placeholder={t('callbackUrlPlaceholder')}
          className="font-mono text-xs"
        />
        <div className="flex items-center gap-2">
          <Button
            type="button"
            onClick={handleSubmitCallback}
            disabled={submitCallback.isPending || !callbackUrl.trim()}
          >
            {submitCallback.isPending ? t('authSubmitting') : t('submitCallback')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setStep('idle')}
          >
            {t('cancel')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      onClick={handleStart}
      disabled={startAuth.isPending}
      className="h-6 px-2 text-xs text-muted-foreground/70 hover:text-muted-foreground hover:bg-transparent"
    >
      {startAuth.isPending ? <Loader2 className="h-3 w-3 mr-1 animate-spin" /> : <Plus className="h-3 w-3 mr-1" />}
      {startAuth.isPending ? t('authStarting') : t('authorize')}
    </Button>
  );
}
