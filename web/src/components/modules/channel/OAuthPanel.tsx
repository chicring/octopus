'use client';

import { useStartAuth, usePollAuth } from '@/api/endpoints/provider';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from '@/components/common/Toast';

interface OAuthPanelProps {
  providerId: string;
  authType: 'oauth_device' | 'oauth_web';
  onSuccess: (accessToken: string) => void;
}

export function OAuthPanel({ providerId, authType, onSuccess }: OAuthPanelProps) {
  const t = useTranslations('provider.form');
  const startAuth = useStartAuth();
  const pollAuth = usePollAuth();
  const [sessionId, setSessionId] = useState<string>('');
  const [userCode, setUserCode] = useState<string>('');
  const [verificationUri, setVerificationUri] = useState<string>('');
  const [polling, setPolling] = useState(false);

  const handleStart = useCallback(async () => {
    const result = await startAuth.mutateAsync({ provider_id: providerId });
    setSessionId(result.session_id);
    if (authType === 'oauth_device') {
      setUserCode(result.user_code || '');
      setVerificationUri(result.verification_uri || '');
      setPolling(true);
    } else {
      // oauth_web: 打开授权页面
      window.open(result.verification_uri, '_blank');
      setPolling(true);
    }
  }, [providerId, authType, startAuth]);

  // 轮询逻辑
  useEffect(() => {
    if (!polling || !sessionId) return;

    const interval = setInterval(async () => {
      try {
        const result = await pollAuth.mutateAsync({ session_id: sessionId });
        if (result.status === 'completed' && result.result?.access_token) {
          setPolling(false);
          onSuccess(result.result.access_token);
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
  }, [polling, sessionId, pollAuth, onSuccess, t]);

  if (polling && authType === 'oauth_device') {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('authDeviceInstruction')}</p>
        <div className="flex items-center gap-2">
          <Input value={userCode} readOnly className="font-mono" />
          <Button
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
        <Button variant="outline" size="sm" onClick={() => window.open(verificationUri, '_blank')}>
          {t('openVerification')}
        </Button>
        <p className="text-xs text-muted-foreground">{t('authPolling')}</p>
      </div>
    );
  }

  if (polling && authType === 'oauth_web') {
    return (
      <div className="space-y-2">
        <p className="text-sm text-muted-foreground">{t('authWebPolling')}</p>
      </div>
    );
  }

  return (
    <Button variant="outline" onClick={handleStart} disabled={startAuth.isPending}>
      {startAuth.isPending ? t('authStarting') : t('authorize')}
    </Button>
  );
}
