/** 从 OAuth 凭证 JSON 中提取显示标签（email > account_id） */
export function parseOAuthLabel(key: string): string | null {
    try {
        const parsed = JSON.parse(key);
        if (parsed && typeof parsed === 'object') {
            return parsed.email || parsed.account_id || null;
        }
    } catch { /* not JSON */ }
    return null;
}
