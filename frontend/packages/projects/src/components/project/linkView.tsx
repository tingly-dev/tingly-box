import { keyframes, type Theme } from '@mui/material/styles';
import { getReadableAccent, type StatusTone } from '@tingly/ui/theme/status';
import type { ThreadLink } from '../../client';
import { IssueOpened, Merge, PullRequest } from '../icons';

export type Tone = Extract<StatusTone, 'success' | 'info' | 'warning' | 'error' | 'muted'>;

export interface Part {
    text: string;
    tone: Tone;
}

export const colorOf = (theme: Theme, tone: Tone) => (tone === 'muted' ? theme.palette.text.secondary : getReadableAccent(theme, tone));

/** Where an item stands: open, merged or closed, and for an open PR its draft
 * mark, checks and mergeability. Only what GitHub last said; a field it was
 * not asked about yet (a PR just opened) is left out, not shown as unknown. */
export function linkParts(link: ThreadLink, t: (key: string) => string): Part[] {
    const parts: Part[] = [];
    if (link.state === 'merged') return [{ text: t('links.merged'), tone: 'info' }];
    if (link.state === 'closed') return [{ text: t('links.closed'), tone: 'muted' }];
    parts.push({ text: link.draft ? t('links.draft') : t('links.open'), tone: link.draft ? 'muted' : 'success' });
    if (link.kind !== 'pr') return parts;
    if (link.ci === 'failing') parts.push({ text: t('links.checksFailing'), tone: 'error' });
    else if (link.ci === 'pending') parts.push({ text: t('links.checksPending'), tone: 'warning' });
    else if (link.ci === 'passing') parts.push({ text: t('links.checksPassing'), tone: 'success' });
    if (link.merge_state === 'conflicts') parts.push({ text: t('links.conflicts'), tone: 'error' });
    else if (link.merge_state === 'behind') parts.push({ text: t('links.behind'), tone: 'warning' });
    else if (link.merge_state === 'blocked') parts.push({ text: t('links.blocked'), tone: 'warning' });
    else if (link.merge_state === 'ready') parts.push({ text: t('links.ready'), tone: 'success' });
    return parts;
}

export function LinkIcon({ link }: { link: ThreadLink }) {
    const sx = { fontSize: 16, flexShrink: 0, mt: '2px' } as const;
    if (link.kind === 'issue') return <IssueOpened sx={{ ...sx, color: (theme) => colorOf(theme, link.state === 'open' ? 'success' : 'muted') }} />;
    if (link.state === 'merged') return <Merge sx={{ ...sx, color: (theme) => colorOf(theme, 'info') }} />;
    const tone: Tone = link.state === 'closed' ? 'error' : link.draft ? 'muted' : 'success';
    return <PullRequest sx={{ ...sx, color: (theme) => colorOf(theme, tone) }} />;
}

/** Text for screen readers only: the live region that says what a refresh did. */
export const visuallyHidden = {
    position: 'absolute', width: 1, height: 1, p: 0, m: '-1px', overflow: 'hidden', clip: 'rect(0 0 0 0)', whiteSpace: 'nowrap', border: 0,
} as const;

/** The turn of a busy refresh icon. */
export const spin = keyframes`from { transform: rotate(0deg); } to { transform: rotate(360deg); }`;
