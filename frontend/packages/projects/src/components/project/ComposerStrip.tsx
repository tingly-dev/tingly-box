import { useState } from 'react';
import { Box, ButtonBase, Collapse, Link, ListItemText, Menu, MenuItem } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread, ThreadLink } from '../../client';
import { CopyIconButton } from '@tingly/ui';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { AutoFixHigh, Branch, Check, ExpandMore, FolderOpen } from '../icons';
import { DisclosureRow } from '../common/Disclosure';
import { ControlTooltip } from '../ControlTooltip';
import { folderName } from '../../utils/format';
import { readLocal, writeLocal } from '../../utils/storage';
import { colorOf, keyParts, LinkIcon, stripLinks } from './linkView';
import { peekRowSx, useLinkPeek } from './useLinkPeek';
import { FilesButton } from './LinksCard';
import { canShowFiles, PrDiffDialog } from './PrDiffDialog';
import { radius } from '@tingly/ui/theme/radius';

export type AutoAnswer = 'off' | 'delay' | 'now';
const AUTO_ANSWERS: AutoAnswer[] = ['off', 'delay', 'now'];
const OPEN_KEY = 'thread.strip.open';

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

/** The most items the open strip lists; the rest are in the Issues & PRs tab. */
const LINKS_SHOWN = 3;
const linkKey = (l: ThreadLink) => `${l.repo}/${l.kind}/${l.number}`;

/** One of the thread's issues or PRs as a line of the open strip: number and title as a link to GitHub, where it stands, and the refresh and details buttons. */
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
    /** The thread sits in a working folder (not a project's thread). */
    folder?: boolean;
    /** Saves the auto-answer setting; the row is left out without it. */
    onAutoAnswer?: (mode: AutoAnswer) => void;
    /** When the question waiting now answers itself, if it will. */
    autoAt?: string;
}

/**
 * What a thread runs in and how it treats its questions, in a panel right
 * above the message box: one fact per line, so a new one (a pull request,
 * CI) is one more line. Folded, it shows one line, the fact that matters
 * most right now, and counts the rest: something about to happen without
 * the user (a question answering itself), then what changes (the branch),
 * and the folder only when there is nothing else. Folded or open is
 * remembered per browser.
 */
export function ComposerStrip({ thread, folder = false, onAutoAnswer, autoAt }: Props) {
    const { t } = useTranslation('projects');
    const [open, setOpenState] = useState(() => readLocal(OPEN_KEY) === '1');
    const setOpen = (v: boolean) => {
        setOpenState(v);
        writeLocal(OPEN_KEY, v ? '1' : '0');
    };
    const [menu, setMenu] = useState<HTMLElement | null>(null);
    const branch = thread.branch || thread.head_branch;
    const mode = autoAnswerOf(thread, folder);
    const own = (thread.auto_answer ?? '') !== '';
    const showAuto = !!onAutoAnswer && thread.status !== 'resolved';
    const links = stripLinks(thread.links);
    const shownLinks = links.slice(0, LINKS_SHOWN);
    if (!thread.cwd && !branch && !showAuto && links.length === 0) return null;

    const rows = [!!thread.cwd, !!branch, showAuto].filter(Boolean).length + shownLinks.length;
    const item = (icon: React.ReactNode, text: React.ReactNode, mono = false, color?: string) => (
        <Box component="span" sx={{ ...clip, display: 'inline-flex', alignItems: 'center', gap: 0.5, color, '& svg': { fontSize: 14, flexShrink: 0 } }}>
            {icon}<Box component="span" sx={{ ...clip, fontFamily: mono ? fontMono : undefined }}>{text}</Box>
        </Box>
    );
    const time = autoAt ? new Date(autoAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '';
    const lead = autoAt && showAuto && mode !== 'off'
        ? item(<AutoFixHigh />, t('strip.autoAt', { time }), false, 'primary.main')
        : links.length > 0
            ? (
                <Box component="span" sx={{ ...clip, display: 'inline-flex', alignItems: 'center', gap: 0.75, '& svg': { fontSize: 14, mt: 0 } }}>
                    <LinkIcon link={links[0]} />
                    <Box component="span" sx={{ flexShrink: 0, color: 'text.primary' }}>#{links[0].number}</Box>
                    {keyParts(links[0], t).map((part) => <Box key={part.text} component="span" sx={{ ...clip, color: (theme) => colorOf(theme, part.tone) }}>{part.text}</Box>)}
                </Box>
            )
        : branch
            ? item(<Branch />, branch, true)
            : thread.cwd ? item(<FolderOpen />, folderName(thread.cwd), true) : null;
    const summary = (
        <>
            {lead}
            {rows > 1 && <Box component="span" sx={{ flexShrink: 0, color: 'text.secondary' }}>+{rows - 1}</Box>}
        </>
    );

    return (
        <Box data-testid="composer-strip" sx={{ mb: 0.75, borderRadius: radius.card, bgcolor: 'action.hover', fontSize: fontSizes.sm, lineHeight: 1.4, color: 'text.secondary', overflow: 'hidden' }}>
            {/* Folded: the one fact that matters now; the whole line unfolds it. */}
            <DisclosureRow
                open={open}
                onToggle={() => setOpen(!open)}
                aria-label={t(open ? 'strip.collapse' : 'strip.expand')}
                sx={{ gap: 1.25, minHeight: ROW_H, px: 1.25, py: 0 }}
            >
                {open
                    ? <Box component="span" sx={{ flex: 1, fontWeight: 600 }}>{t(folder ? 'strip.sessionTitle' : 'strip.title')}</Box>
                    : <Box component="span" sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 0.75, overflow: 'hidden' }}>{summary}</Box>}
            </DisclosureRow>
            <Collapse in={open} unmountOnExit>
                <Box sx={{ pb: 0.5 }}>
                    {thread.cwd && (
                        <Row icon={<FolderOpen />} name={t('strip.folder')}
                            action={<CopyIconButton value={`cd ${thread.cwd}`} label={t('strip.copyCd')} copiedLabel={t('common.copied')} iconSize={14} sx={{ p: 0.25 }} />}>
                            {/* A long path keeps its end, the part that tells folders apart. */}
                            <Box component="span" title={thread.cwd} sx={{ ...clip, fontFamily: fontMono, direction: 'rtl', textAlign: 'left' }}><bdi>{thread.cwd}</bdi></Box>
                        </Row>
                    )}
                    {branch && (
                        <Row icon={<Branch />} name={t('strip.branch')}
                            action={<CopyIconButton value={branch} label={t('strip.copyBranch')} copiedLabel={t('common.copied')} iconSize={14} sx={{ p: 0.25 }} />}>
                            <Box component="span" sx={{ ...clip, fontFamily: fontMono }}>{branch}</Box>
                            {thread.branch && thread.base_branch && <Box component="span" sx={{ ...clip, flexShrink: 0, color: 'text.secondary' }}>{t('thread.fromBase', { base: thread.base_branch })}</Box>}
                        </Row>
                    )}
                    {shownLinks.map((l) => <LinkLine key={linkKey(l)} link={l} thread={thread} />)}
                    {links.length > shownLinks.length && <Box sx={{ px: 1.25, minHeight: ROW_H, display: 'flex', alignItems: 'center' }}>{t('strip.moreLinks', { count: links.length - shownLinks.length })}</Box>}
                    {showAuto && (
                        <Row icon={<AutoFixHigh sx={{ color: mode === 'off' ? undefined : 'primary.main' }} />} name={t('strip.autoAnswer.title')}>
                            <ControlTooltip title={t(`strip.autoAnswer.${mode}Help`)} suppressed={!!menu}>
                                <ButtonBase onClick={(e) => setMenu(e.currentTarget)} aria-label={t('strip.autoAnswer.change')}
                                    sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25, px: 0.75, ml: -0.75, py: 0.25, borderRadius: radius.row, font: 'inherit', color: 'text.primary', '&:hover': { bgcolor: 'action.selected' } }}>
                                    {t(`strip.autoAnswer.${mode}`)}
                                    <ExpandMore sx={{ fontSize: 15, color: 'text.secondary' }} />
                                </ButtonBase>
                            </ControlTooltip>
                        </Row>
                    )}
                </Box>
            </Collapse>
            {onAutoAnswer && (
                <Menu anchorEl={menu} open={!!menu} onClose={() => setMenu(null)}
                    anchorOrigin={{ vertical: 'top', horizontal: 'right' }} transformOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                    slotProps={{ paper: { sx: { maxWidth: 340 } } }}>
                    <Box sx={{ px: 2, pt: 0.5, pb: 0.75, fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('strip.autoAnswer.menuTitle')}</Box>
                    {AUTO_ANSWERS.map((m) => (
                        <MenuItem key={m} selected={m === mode} onClick={() => { setMenu(null); if (m !== mode || !own) onAutoAnswer(m); }} sx={{ alignItems: 'flex-start', gap: 1 }}>
                            <Box sx={{ width: 16, pt: 0.25, flexShrink: 0 }}>{m === mode && <Check sx={{ fontSize: 16 }} />}</Box>
                            <ListItemText primary={t(`strip.autoAnswer.${m}`)} secondary={t(`strip.autoAnswer.${m}Help`)}
                                slotProps={{ primary: { sx: { fontSize: fontSizes.base } }, secondary: { sx: { whiteSpace: 'normal', fontSize: fontSizes.sm } } }} />
                        </MenuItem>
                    ))}
                </Menu>
            )}
        </Box>
    );
}
