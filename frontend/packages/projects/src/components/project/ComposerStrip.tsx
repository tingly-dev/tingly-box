import { useState } from 'react';
import { Box, Link } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread, ThreadLink } from '../../client';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { colorOf, keyParts, LinkIcon, stripLinks } from './linkView';
import { peekRowSx, useLinkPeek } from './useLinkPeek';
import { FilesButton } from './LinksCard';
import { canShowFiles, PrDiffDialog } from './PrDiffDialog';
import { radius } from '@tingly/ui/theme/radius';
import { mobile } from '@tingly/ui/theme/mobile';
import { DisclosureRow } from '../common/Disclosure';
import { useGroupFolds } from '../../hooks/useGroupFolds';

const clip = { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0 } as const;
const ROW_H = 30;

/** One fact on its own line: icon, name, value, and its action at the end. */
function Row({ icon, name, children, action, boxProps }: { icon: React.ReactNode; name: string; children: React.ReactNode; action?: React.ReactNode; boxProps?: object }) {
    return (
        <Box {...boxProps} sx={{ display: 'flex', alignItems: 'center', gap: 1, minHeight: ROW_H, px: 1.25, minWidth: 0, '& > svg': { fontSize: 15, flexShrink: 0, color: 'text.secondary' }, ...(boxProps ? peekRowSx : {}) }}>
            {icon}
            <Box component="span" sx={{ width: 84, flexShrink: 0, color: 'text.secondary', whiteSpace: 'nowrap' }}>{name}</Box>
            <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 0.75, color: 'text.primary' }}>{children}</Box>
            {action}
        </Box>
    );
}

const linkKey = (l: ThreadLink) => `${l.repo}/${l.kind}/${l.number}`;

/** One of the thread's issues or PRs as a line of the strip: number and title as a link to GitHub, where it stands, and the refresh and details buttons. */
function LinkLine({ link, thread }: { link: ThreadLink; thread: Thread }) {
    const { t } = useTranslation('projects');
    const [files, setFiles] = useState(false);
    const threadId = thread.id;
    // The strip sits above the message box, at the bottom of the screen: the details open upward.
    const peek = useLinkPeek(link, threadId, true);
    return (
        <Row icon={<Box component="span" sx={{ display: 'inline-flex', '& svg': { mt: 0 } }}><LinkIcon link={link} /></Box>} name={t(link.kind === 'pr' ? 'strip.pr' : 'strip.issue')}
            action={canShowFiles(link) ? <Box sx={{ display: 'flex', alignItems: 'center' }}><FilesButton link={link} onClick={() => setFiles(true)} />{peek.actions}</Box> : peek.actions} boxProps={peek.row}>
            {/* The title keeps a readable width and the state wraps under it where the strip is narrow (a phone, with its big touch buttons). */}
            <Box sx={{ flex: 1, minWidth: 0, display: 'flex', flexWrap: 'wrap', alignItems: 'center', columnGap: 0.75 }}>
                <Link href={link.url.startsWith('https://') ? link.url : undefined} target="_blank" rel="noopener noreferrer" underline="hover" color="text.primary" title={`${link.repo}#${link.number} ${link.title}`}
                    sx={{ ...clip, flex: '1 1 7rem' }}>
                    #{link.number} {link.title}
                </Link>
                {keyParts(link, t).map((part) => <Box key={part.text} component="span" sx={{ flexShrink: 0, color: (theme) => colorOf(theme, part.tone) }}>{part.text}</Box>)}
            </Box>
            {peek.overlay}
            {files && <PrDiffDialog thread={thread} link={link} open onClose={() => setFiles(false)} />}
        </Row>
    );
}

interface Props {
    thread: Thread;
}

/** The folded strip's one-line summary: how many are listed, how many PRs are open, and a red dot when one of those has failing checks or conflicts. */
function Summary({ links }: { links: ThreadLink[] }) {
    const { t } = useTranslation('projects');
    const openPrs = links.filter((l) => l.kind === 'pr' && l.state === 'open');
    const broken = openPrs.some((l) => l.ci === 'failing' || l.merge_state === 'conflicts');
    return (
        <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, fontWeight: 400, minWidth: 0 }}>
            <span>{links.length}</span>
            {openPrs.length > 0 && <span>· {t('strip.openPrs', { count: openPrs.length })}</span>}
            {broken && <Box component="span" role="img" aria-label={t('strip.needsLook')} title={t('strip.needsLook')} sx={{ width: 8, height: 8, borderRadius: '50%', flexShrink: 0, bgcolor: 'error.main' }} />}
        </Box>
    );
}

/**
 * The thread's issues and PRs, in a panel right above the message box: one
 * line each, all of them listed. The title row folds the whole panel to one
 * line (the choice is kept for the browser, not per thread); open, the list
 * scrolls inside a capped height so many links never crowd out the
 * conversation. Where the thread runs (folder, branch, session) is the
 * details rail's, and how it treats its questions is a picker in the message
 * box: this panel repeats neither. Without a link it is not there.
 */
export function ComposerStrip({ thread }: Props) {
    const { t } = useTranslation('projects');
    const [folds, setFolds] = useGroupFolds('composer-strip');
    const links = stripLinks(thread.links);
    if (links.length === 0) return null;
    const open = !folds.links;

    return (
        <Box data-testid="composer-strip" sx={{ mb: 0.75, py: 0.5, borderRadius: radius.card, bgcolor: 'action.hover', fontSize: fontSizes.sm, lineHeight: 1.4, color: 'text.secondary', overflow: 'hidden' }}>
            <DisclosureRow open={open} onToggle={() => setFolds(['links'], open)} variant="header" aria-controls="composer-strip-body"
                sx={{ px: 1.25, py: 0, minHeight: ROW_H, fontWeight: 600, '@media (pointer: coarse)': { minHeight: mobile.tapMin } }}>
                <span>{t('strip.title')}</span>
                {!open && <Summary links={links} />}
            </DisclosureRow>
            {open && (
                <Box id="composer-strip-body" sx={{ maxHeight: `calc(${ROW_H}px * 5.5)`, overflowY: 'auto' }}>
                    {links.map((l) => <LinkLine key={linkKey(l)} link={l} thread={thread} />)}
                </Box>
            )}
        </Box>
    );
}
