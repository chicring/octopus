import { apiClient } from '@/api/client';
import { useQuery, useMutation } from '@tanstack/react-query';

// Provider 信息
export interface ProviderInfo {
  id: string;
  display_name: string;
  auth_type: string;
  supports_chat: boolean;
  supports_embedding: boolean;
  credential_schema?: CredentialSchema;
}

// Provider 预设
export interface ProviderPreset {
  name: string;
  provider_id: string;
  default_base_url: string;
  auth_type: string;
}

// 凭证 Schema
export interface CredentialSchema {
  auth_type: string;
  fields: CredentialField[];
}

// 凭证字段
export interface CredentialField {
  key: string;
  label: string;
  type: 'text' | 'password' | 'url' | 'hidden';
  required: boolean;
  default?: string;
  placeholder?: string;
  secret: boolean;
  order: number;
}

// Auth Session
export interface AuthSession {
  session_id: string;
  user_code?: string;
  verification_uri?: string;
  expires_at?: number;
  interval?: number;
}

// Auth Result
export interface AuthResult {
  status: 'pending' | 'completed' | 'failed';
  result?: {
    access_token: string;
    token_type?: string;
    expires_in?: number;
    refresh_token?: string;
    scope?: string;
    extra?: Record<string, string>;
  };
}

// Hooks

export function useProviderList() {
  return useQuery({
    queryKey: ['providers', 'list'],
    queryFn: async () => apiClient.get<ProviderInfo[]>('/api/v1/provider/list'),
    staleTime: 5 * 60 * 1000,
  });
}

export function useProviderPresets() {
  return useQuery({
    queryKey: ['providers', 'presets'],
    queryFn: async () => apiClient.get<ProviderPreset[]>('/api/v1/provider/presets'),
    staleTime: 5 * 60 * 1000,
  });
}

export function useStartAuth() {
  return useMutation({
    mutationFn: async (data: { provider_id: string; channel_id?: number }) =>
      apiClient.post<AuthSession>('/api/v1/provider/auth/start', data),
  });
}

export function usePollAuth() {
  return useMutation({
    mutationFn: async (data: { session_id: string }) =>
      apiClient.post<AuthResult>('/api/v1/provider/auth/poll', data),
  });
}

export function useSubmitCallback() {
  return useMutation({
    mutationFn: async (data: { session_id: string; callback_url: string }) =>
      apiClient.post<NonNullable<AuthResult['result']>>('/api/v1/provider/auth/submit-callback', data),
  });
}
