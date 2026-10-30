// DESIGN DRAFT ONLY (not for commit): ?draft=1|2|3 layers the routing-graph proposals.
export const draftLevel = (): number => {
    try { return Number(new URLSearchParams(window.location.search).get('draft') || 0); } catch { return 0; }
};
const zh = () => (localStorage.getItem('i18nextLng') || 'zh').startsWith('zh');
export const draftOpText = (position: string, operation: string, value: string): string | null => {
    const z = zh();
    if (position === 'agent.claude_code' && value === 'subagent') return z ? 'Claude Code 子代理发出的请求' : 'Request from a Claude Code subagent';
    if (position === 'agent.claude_code' && value === 'compact') return z ? 'Claude Code 压缩上下文时' : 'Claude Code compacting context';
    if (position === 'token' && operation === 'ge') return z ? `上下文 ≥ ${Number(value) / 1000}k tokens` : `Context ≥ ${Number(value) / 1000}k tokens`;
    return null;
};
export const ONE = draftLevel() >= 4;
export const ONE_H = 36;
export const DRAFT_HIT_RULE = 'smart-cc-bi-tok';
export const DRAFT_HIT_SERVICE = 'svc-sm-cc-bi-tok';
export const DRAFT_PAUSED_SERVICE = 'svc-sm-cc-bi-cmp';
export const draftText = (k: 'hit' | 'hitSvc' | 'paused' | 'otherwise' | 'otherwiseSub') => {
    const z = zh();
    return {
        hit: z ? '最近命中 · 2 分钟前' : 'Last hit · 2 min ago',
        hitSvc: z ? '最近一次由它回答' : 'Answered the last request',
        paused: z ? '暂停中 · 约 30 秒后重试' : 'Paused · retry in ~30s',
        otherwise: z ? '否则' : 'Otherwise',
        otherwiseSub: z ? '以上条件都不满足' : 'none above matched',
    }[k];
};
