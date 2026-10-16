import {getReadableAccent} from '@/theme/status';
import {useTheme, type Theme} from '@mui/material/styles';
import {Box, Tooltip, Typography} from '@mui/material';
import type {ReactNode} from 'react';
import {useTranslation} from 'react-i18next';
import {formatTokens} from '@tingly/projects/transcript';
import {cacheHitPct, type SessionUsage} from './sessionUsage';
import { fontMono, fontSizes } from '@/theme/fonts';

// Where a session's requests are routed and the quota they draw on: the
// Tingly Box half of the line (GET /scenario/claude_code/route, or Desk's own
// status route, which answers the same).
export interface RouteQuota {
    type: string;
    balance: boolean;
    text: string;
    used_percent: number;
    resets_at?: string;
    limit_reached: boolean;
}

export interface RouteStatus {
    scenario: string;
    requested_model?: string;
    provider_name?: string;
    provider_model?: string;
    quota: RouteQuota[];
}

// The same 8-cell bar the terminal status line draws for context use.
const contextBar = (pct: number): string => {
    const filled = Math.min(Math.max(Math.round((pct * 8) / 100), 0), 8);
    return '▓'.repeat(filled) + '░'.repeat(8 - filled);
};

const resetsIn = (iso: string | undefined, now = Date.now()): string | undefined => {
    if (!iso) return undefined;
    const mins = Math.round((new Date(iso).getTime() - now) / 60_000);
    if (mins <= 0) return undefined;
    if (mins < 60) return `${mins}m`;
    const h = Math.floor(mins / 60);
    if (h < 48) return mins % 60 ? `${h}h ${mins % 60}m` : `${h}h`;
    return `${Math.round(h / 24)}d`;
};

const quotaColor = (theme: Theme, q: RouteQuota): string | undefined => {
    if (q.limit_reached || q.used_percent >= 90) return getReadableAccent(theme, 'error');
    if (q.used_percent >= 80) return getReadableAccent(theme, 'warning');
    return undefined;
};

const Segment = ({children, tip, color}: {children: ReactNode; tip?: ReactNode; color?: string}) => {
    const body = (
        <Box component="span" sx={{color: color ?? 'inherit', whiteSpace: 'nowrap', cursor: tip ? 'help' : undefined}}>
            {children}
        </Box>
    );
    return tip ? <Tooltip title={tip}>{body}</Tooltip> : body;
};

interface RouteStatusLineProps {
    status?: RouteStatus;
    usage?: SessionUsage;
}

// RouteStatusLine is the web counterpart of the status line tingly-box
// installs for Claude Code in a terminal (internal/server/module/statusline):
// the model a turn asked for and where tingly-box routed it, how full the
// context is, the session's tokens, and the routed provider's quota.
const RouteStatusLine = ({status, usage}: RouteStatusLineProps) => {
    const {t} = useTranslation();
    const theme = useTheme();
    const segments: ReactNode[] = [];
    const model = status?.requested_model || usage?.latest.model;
    if (model) {
        const routed = status?.provider_model
            ? <> → {status.provider_model} @ {status.provider_name}</>
            : null;
        segments.push(
            <Segment key="route" tip={status ? t('desk.statusRoute', {defaultValue: 'Routed by the {{scenario}} rules', scenario: status.scenario}) : undefined}>
                {model}{routed}
            </Segment>,
        );
    }
    if (usage) {
        const {context_tokens: ctx, context_window: win} = usage.latest;
        if (win) {
            const pct = Math.round((ctx / win) * 100);
            segments.push(
                <Segment key="ctx" color={pct >= 80 ? getReadableAccent(theme, 'warning') : undefined}
                    tip={t('desk.statusContext', {defaultValue: 'Context: {{used}} of {{window}} tokens', used: formatTokens(ctx), window: formatTokens(win)})}>
                    {contextBar(pct)} {pct}%
                </Segment>,
            );
        } else if (ctx > 0) {
            segments.push(<Segment key="ctx">ctx {formatTokens(ctx)}</Segment>);
        }
        segments.push(
            <Segment key="tokens" tip={t('desk.statusTokens', {defaultValue: 'This session, as returned through the gateway: input (including cache) ↑, output ↓'})}>
                ↑{formatTokens(usage.input + usage.cacheRead + usage.cacheWrite)} ↓{formatTokens(usage.output)} · cache {cacheHitPct(usage)}%
            </Segment>,
        );
    }
    const quotas = status?.quota.filter((q) => !q.balance) ?? [];
    for (const q of quotas) {
        const reset = resetsIn(q.resets_at);
        segments.push(
            <Segment key={`q-${q.type}`} color={quotaColor(theme, q)}
                tip={[`${q.type} ${t('desk.statusQuota', {defaultValue: 'quota'})}`, reset && t('desk.statusResets', {defaultValue: 'resets in {{reset}}', reset})].filter(Boolean).join(' · ')}>
                {q.type} {q.text}
            </Segment>,
        );
    }
    const balances = status?.quota.filter((q) => q.balance) ?? [];
    if (balances.length > 0) {
        segments.push(<Segment key="balance">{t('desk.statusBalance', {defaultValue: 'Balance'})} {balances.map((b) => b.text).join(' · ')}</Segment>);
    }
    if (quotas.some((q) => q.limit_reached)) {
        segments.push(
            <Segment key="exhausted" color={getReadableAccent(theme, 'error')}>
                {t('desk.statusExhausted', {defaultValue: 'quota exhausted — pick another profile above'})}
            </Segment>,
        );
    }

    if (segments.length === 0) return null;
    return (
        <Typography
            component="div"
            variant="caption"
            sx={{
                mt: 0.75, px: 1.5, color: 'text.secondary', fontFamily: fontMono, fontSize: fontSizes.xs,
                display: 'flex', flexWrap: 'wrap', columnGap: 1, rowGap: 0.25,
                '& > *:not(:last-child)::after': {content: '"|"', ml: 1, color: 'text.disabled'},
            }}
        >
            {segments}
        </Typography>
    );
};

export default RouteStatusLine;
