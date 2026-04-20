'use client';

import { useProviderPresets } from '@/api/endpoints/provider';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useTranslations } from 'next-intl';

interface ProviderPresetSelectProps {
  value: string;
  onValueChange: (value: string, preset: { provider_id: string; default_base_url: string }) => void;
}

export function ProviderPresetSelect({ value, onValueChange }: ProviderPresetSelectProps) {
  const t = useTranslations('provider.form');
  const { data: presets } = useProviderPresets();

  return (
    <Select
      value={value}
      onValueChange={(v) => {
        const preset = presets?.find((p) => p.name === v);
        if (preset) {
          onValueChange(v, { provider_id: preset.provider_id, default_base_url: preset.default_base_url });
        }
      }}
    >
      <SelectTrigger>
        <SelectValue placeholder={t('selectPreset')} />
      </SelectTrigger>
      <SelectContent>
        {presets?.map((preset) => (
          <SelectItem key={preset.name} value={preset.name}>
            {preset.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
