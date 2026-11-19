import type { ReactNode } from 'react';
import { Box, Button, Link, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { ThreadLink } from '../../client';
import { useCopyFeedback } from '@tingly/ui';
import { timeAgo } from '../../utils/format';
import { Check, Close, ContentCopy, OpenInNew, Refresh } from '../icons';
import { colorOf, LinkIcon, type Tone } from './linkView';

const isHttps = (url: string | undefined): url is string => !!url && url.startsWith('https://');

/** Where an item stands as one word: open, draft, merged or closed. */
function stateWord(link: ThreadLink, t: (key: string) => string): { text: string; tone: Tone } {
    if (link.state === 'merged') return { text: t('links.merged'), tone: 'info' };
    if (link.state === 'closed') return { text: t('links.closed'), tone: 'muted' };
    return link.draft ? { text: t('links.draft'), tone: 'muted' } : { text: t('links.open'), tone: 'success' };
}

function Field({ label, children }: { label: string; children: ReactNode }) {
    return (
        <>
            <Typography component="dt" sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{label}</Typography>
            <Typography component="dd" sx={{ m: 0, minWidth: 0, fontSize: fontSizes.sm, color: 'text.primary', overflowWrap: 'anywhere' }}>{children}</Typography>
        </>
    );
}

/**
 * What a row cannot hold, and only what a person acts on: the full title, the
 * checks (the failing ones by name), the merge state and when GitHub was last
 * read. A field the record does not have yet is left out, never shown as
 * unknown. The row's refresh, plus the two a row has no room for: open on
 * GitHub and copy the link.
 */
export function LinkDetails({ link, busy, outcome, onRefresh, fill = false }: { link: ThreadLink; busy: boolean; outcome: string; onRefresh?: () => void; /** Take the width of the sheet it is in, instead of a popover's. */ fill?: boolean }) {
    const { t, i18n } = useTranslation('projects');
    const { copied, copy } = useCopyFeedback();
    const state = stateWord(link, t);
    const checks = link.checks;
    const counts = checks && [
        checks.failing > 0 && { text: t('links.checkFailing', { count: checks.failing }), tone: 'error' as Tone },
        checks.pending > 0 && { text: t('links.checkPending', { count: checks.pending }), tone: 'warning' as Tone },
        checks.passing > 0 && { text: t('links.checkPassing', { count: checks.passing }), tone: 'success' as Tone },
    ].filter((c): c is { text: string; tone: Tone } => !!c);
    const mergeText = link.merge_state && t(`links.merge.${link.merge_state}`);
    const open = link.state === 'open' && link.kind === 'pr';
    const read = timeAgo(link.refreshed_at, i18n.language);

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, p: 1.5, width: fill ? '100%' : 360, maxWidth: '100%', boxSizing: 'border-box' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0 }}>
                <LinkIcon link={link} />
                <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{link.repo}#{link.number}</Typography>
                <Box sx={{ flex: 1 }} />
                <Typography component="span" sx={{ fontSize: fontSizes.sm, fontWeight: 600, color: (theme) => colorOf(theme, state.tone) }}>{state.text}</Typography>
            </Box>
            <Typography sx={{ fontSize: fontSizes.base, fontWeight: 600, color: 'text.primary', overflowWrap: 'anywhere' }}>{link.title}</Typography>
            <Box component="dl" sx={{ m: 0, display: 'grid', gridTemplateColumns: 'max-content 1fr', columnGap: 1.5, rowGap: 0.5 }}>
                {open && counts && counts.length > 0 && (
                    <Field label={t('links.checks')}>
                        <Box component="span" sx={{ display: 'inline-flex', flexWrap: 'wrap', columnGap: 1 }}>
                            {counts.map((c) => <Box key={c.text} component="span" sx={{ color: (theme) => colorOf(theme, c.tone) }}>{c.text}</Box>)}
                        </Box>
                        {checks && checks.failed && checks.failed.length > 0 && (
                            <Box component="ul" aria-label={t('links.checkFailingList')} sx={{ m: 0, mt: 0.25, p: 0, listStyle: 'none' }}>
                                {checks.failed.map((c) => (
                                    <Box component="li" key={c.name} sx={{ display: 'flex', gap: 0.75, minWidth: 0 }}>
                                        <Close role="img" aria-label={t('links.checkFailingOne')} sx={{ fontSize: 14, flexShrink: 0, mt: '2px', color: (theme) => colorOf(theme, 'error') }} />
                                        {isHttps(c.url)
                                            ? <Link href={c.url} target="_blank" rel="noopener noreferrer" underline="hover" color="text.primary" sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>{c.name}</Link>
                                            : <Box component="span" sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>{c.name}</Box>}
                                    </Box>
                                ))}
                                {(checks.more ?? 0) > 0 && <Box component="li" sx={{ color: 'text.secondary' }}>{t('links.checksMore', { count: checks.more })}</Box>}
                            </Box>
                        )}
                    </Field>
                )}
                {open && mergeText && <Field label={t('links.merge.label')}>{mergeText}</Field>}
                <Field label={t('links.read')}>{read}</Field>
            </Box>
            {link.last_error && (
                <Typography role="alert" sx={{ fontSize: fontSizes.sm, color: (theme) => colorOf(theme, 'warning'), overflowWrap: 'anywhere' }}>{t('links.stale', { age: read })} — {link.last_error}</Typography>
            )}
            <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 0.75, pt: 0.5, borderTop: 1, borderColor: 'divider' }}>
                {onRefresh && (
                    <Button size="small" onClick={onRefresh} disabled={busy} aria-busy={busy} startIcon={<Refresh fontSize="small" />}>{busy ? t('links.refreshing') : t('links.refresh')}</Button>
                )}
                {isHttps(link.url) && (
                    <Button size="small" component="a" href={link.url} target="_blank" rel="noopener noreferrer" startIcon={<OpenInNew fontSize="small" />}>{t('links.openOnGitHub')}</Button>
                )}
                {isHttps(link.url) && (
                    <Button size="small" onClick={() => copy(link.url)} startIcon={copied ? <Check fontSize="small" /> : <ContentCopy fontSize="small" />}>{copied ? t('links.copied') : t('links.copyLink')}</Button>
                )}
                {outcome && <Typography role="status" sx={{ fontSize: fontSizes.sm, color: 'text.secondary', ml: 'auto' }}>{outcome}</Typography>}
            </Box>
        </Box>
    );
}
