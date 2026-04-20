'use client';

import type { CredentialField } from '@/api/endpoints/provider';
import { Input } from '@/components/ui/input';

interface DynamicCredentialFieldProps {
  field: CredentialField;
  value: string;
  onChange: (key: string, value: string) => void;
}

export function DynamicCredentialField({ field, value, onChange }: DynamicCredentialFieldProps) {
  const inputType = field.type === 'password' ? 'password' : field.type === 'url' ? 'url' : 'text';

  return (
    <Input
      type={inputType}
      value={value}
      placeholder={field.placeholder}
      onChange={(e) => onChange(field.key, e.target.value)}
    />
  );
}
