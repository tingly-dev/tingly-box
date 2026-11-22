import { useState } from 'react';
import { Box, ButtonBase, Link, ListItemText, Menu, MenuItem } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread, ThreadLink } from '../../client';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { AutoFixHigh, Check, ExpandMore } from '../icons';
import { ControlTooltip } from '../ControlTooltip';
import { colorOf, keyParts, LinkIcon, stripLinks } from './linkView';
import { peekRowSx, useLinkPeek } from './useLinkPeek';
import { FilesButton } from './LinksCard';
import { canShowFiles, PrDiffDialog } from './PrDiffDialog';
import { radius } from '@tingly/ui/theme/radius';

export type AutoAnswer = 'off' | 'delay' | 'now';
const AUTO_ANSWERS: AutoAnswer[] = ['off', 'delay', 'now'];

/**
 * The setting a thread's questions follow: its own, else delay for a
 * project thread (its owner is usually away) and off for a folder session
 * (its owner is usually watching). Mirrors the server's autoAnswerMode.
 */
export function autoAnswerOf(thread: Thread, folder = false): AutoAnswer {
    const own = thread.auto_answer as AutoAnswer | undefined;
    if (own && AUTO_ANSWERS.includes(own)) return own;
    return folder ? 'off' : 'delay';
}

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

/** The auto-answer setting as a compact button that opens the three choices. */
export function AutoAnswerPicker({ mode, own, onChange }: { mode: AutoAnswer; own: boolean; onChange: (mode: AutoAnswer) => void }) {
    const { t } = useTranslation('projects');
    const [menu, setMenu] = useState<HTMLElement | null>(null);
    return (
        <>
            <ControlTooltip title={t(`strip.autoAnswer.${mode}Help`)} suppressed={!!menu}>
                <ButtonBase onClick={(e) => setMenu(e.currentTarget)} aria-label={t('strip.autoAnswer.change')}
                    sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25, px: 0.75, ml: -0.75, py: 0.25, borderRadius: radius.row, font: 'inherit', color: 'text.primary', '&:hover': { bgcolor: 'action.selected' } }}>
                    {t(`strip.autoAnswer.${mode}`)}
                    <ExpandMore sx={{ fontSize: 15, color: 'text.secondary' }} />
                </ButtonBase>
            </ControlTooltip>
            <Menu anchorEl={menu} open={!!menu} onClose={() => setMenu(null)}
                anchorOrigin={{ vertical: 'top', horizontal: 'right' }} transformOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                slotProps={{ paper: { sx: { maxWidth: 340 } } }}>
                <Box sx={{ px: 2, pt: 0.5, pb: 0.75, fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('strip.autoAnswer.menuTitle')}</Box>
                {AUTO_ANSWERS.map((m) => (
                    <MenuItem key={m} selected={m === mode} onClick={() => { setMenu(null); if (m !== mode || !own) onChange(m); }} sx={{ alignItems: 'flex-start', gap: 1 }}>
                        <Box sx={{ width: 16, pt: 0.25, flexShrink: 0 }}>{m === mode && <Check sx={{ fontSize: 16 }} />}</Box>
                        <ListItemText primary={t(`strip.autoAnswer.${m}`)} secondary={t(`strip.autoAnswer.${m}Help`)}
                            slotProps={{ primary: { sx: { fontSize: fontSizes.base } }, secondary: { sx: { whiteSpace: 'normal', fontSize: fontSizes.sm } } }} />
                    </MenuItem>
                ))}
            </Menu>
        </>
    );
}

interface Props {
    thread: Thread;
    /** The thread sits in a working folder (not a project's thread). */
    folder?: boolean;
    /** Saves the auto-answer setting; the row is left out without it. */
    onAutoAnswer?: (mode: AutoAnswer) => void;
}

/**
 * The thread's issues and PRs, in a panel right above the message box: one
 * line each, all of them listed. Where the thread runs (folder, branch,
 * session) is the details rail's; this panel does not repeat it.
 */
export function ComposerStrip({ thread, folder = false, onAutoAnswer }: Props) {
    const { t } = useTranslation('projects');
    const mode = autoAnswerOf(thread, folder);
    const own = (thread.auto_answer ?? '') !== '';
    const showAuto = !!onAutoAnswer && thread.status !== 'resolved';
    const links = stripLinks(thread.links);
    if (!showAuto && links.length === 0) return null;

    return (
        <Box data-testid="composer-strip" sx={{ mb: 0.75, py: 0.5, borderRadius: radius.card, bgcolor: 'action.hover', fontSize: fontSizes.sm, lineHeight: 1.4, color: 'text.secondary', overflow: 'hidden' }}>
            {links.length > 0 && <Box sx={{ px: 1.25, minHeight: ROW_H, display: 'flex', alignItems: 'center', fontWeight: 600 }}>{t('strip.title')}</Box>}
            {links.map((l) => <LinkLine key={linkKey(l)} link={l} thread={thread} />)}
            {showAuto && onAutoAnswer && (
                <Row icon={<AutoFixHigh sx={{ color: mode === 'off' ? undefined : 'primary.main' }} />} name={t('strip.autoAnswer.title')}>
                    <AutoAnswerPicker mode={mode} own={own} onChange={onAutoAnswer} />
                </Row>
            )}
        </Box>
    );
}
