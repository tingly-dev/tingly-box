import { useCallback, useMemo, useState } from 'react';
import { Box, Button, Link, Skeleton, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Typography } from '@mui/material';
import { EmptyState, PageHeader, Surface } from '@tingly/ui';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { MoveChatDialog } from '../components/im/MoveChatDialog';
import { PlatformMark, usePlatformLabel } from '../components/im/PlatformMark';
import { isFolder } from '../client';
import { chatKey, useChannelBindings, useImChats, useProjectDirectory } from '../components/im/useImBindings';
import { ChatBubble } from '../components/icons';
import { useNotify } from '../hooks/useNotify';
import { useProjectsHost, type ImBot, type ImChat } from '../host/ProjectsHost';

const STATE_COLOR = { ok: 'text.secondary', blocked: 'text.disabled', unpaired: 'warning.main' } as const;

function Dot({ color }: { color: string }) {
    return <Box component="span" aria-hidden="true" sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: color, flexShrink: 0 }} />;
}

/**
 * IM settings (the sidebar's "IM" entry): each bot with the chats it has seen,
 * where each one is bound, and the other side of "bind from either side" —
 * binding or moving a chat to a project. Bot connection and access control
 * stay the host's (the "Access control" button opens its dialog).
 */
export default function ImSettingsPage() {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const { im, paths, embedded } = useProjectsHost();
    const platformLabel = usePlatformLabel();
    const { bots, chats, reload } = useImChats(!!im);
    const { bindings, byChat, bind } = useChannelBindings(!!im);
    const directory = useProjectDirectory(!!im);
    // A chat is bound to a project from here; a folder is bound by one of its threads, from its page.
    const bindable = useMemo(() => [...directory.values()].filter((p) => !isFolder(p)), [directory]);
    const [accessBot, setAccessBot] = useState<ImBot | null>(null);
    const [moving, setMoving] = useState<ImChat | null>(null);

    const chatsByBot = useMemo(() => {
        const m = new Map<string, ImChat[]>();
        for (const c of chats ?? []) m.set(c.botUuid, [...(m.get(c.botUuid) ?? []), c]);
        return m;
    }, [chats]);

    const onMove = useCallback(async (chat: ImChat, projectId: string) => {
        try {
            await bind(chat, { project_id: projectId });
            notify(t('im.settings.moveDone', { chat: chat.title, project: directory.get(projectId)?.name ?? '' }), 'success');
        } catch (e) {
            notify(`${t('im.bind.failed')}: ${(e as Error).message}`, 'error');
            throw e;
        }
    }, [bind, notify, directory, t]);

    const loading = bots === null || chats === null || bindings === null;

    return (
        <Box sx={{ flex: 1, overflowY: 'auto' }}>
            <Box sx={{ maxWidth: 1040, mx: 'auto', px: { xs: 2, md: embedded ? 3 : 4 }, pt: embedded ? { xs: 2, md: 3 } : { xs: 7, md: 5 }, pb: 6 }}>
                <PageHeader title={t('im.settings.title')} subtitle={t('im.settings.intro')} icon={<ChatBubble />} sx={{ mb: 3 }} />

                {loading ? (
                    <Skeleton variant="rounded" height={180} />
                ) : bots.length === 0 ? (
                    <EmptyState icon={<ChatBubble sx={{ fontSize: 40 }} />} title={t('im.settings.emptyTitle')} description={t('im.settings.emptyBody')} />
                ) : (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        {bots.map((bot) => {
                            const rows = chatsByBot.get(bot.uuid) ?? [];
                            return (
                                <Surface key={bot.uuid} padding={0} sx={{ overflow: 'hidden' }}>
                                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.25, px: 2, py: 1.5, borderBottom: rows.length > 0 ? 1 : 0, borderColor: 'divider' }}>
                                        <PlatformMark platform={bot.platform} size={24} />
                                        <Box sx={{ minWidth: 0, flex: 1, display: 'flex', alignItems: 'baseline', columnGap: 1.5, rowGap: 0.25, flexWrap: 'wrap' }}>
                                            <Typography noWrap sx={{ fontWeight: 600 }}>{bot.name}</Typography>
                                            <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary', display: 'inline-flex', alignItems: 'center', gap: 0.75 }}>
                                                <Dot color={bot.online ? 'text.secondary' : 'text.disabled'} />
                                                {platformLabel(bot.platform)} · {bot.online ? t('im.settings.online') : t('im.settings.offline')}
                                            </Typography>
                                        </Box>
                                        {im?.accessDialog && (
                                            <Button size="small" variant="outlined" color="inherit" onClick={() => setAccessBot(bot)}>{t('im.settings.access')}</Button>
                                        )}
                                    </Box>
                                    {rows.length === 0 ? (
                                        <Typography color="text.secondary" sx={{ px: 2, py: 2, fontSize: fontSizes.base }}>{t('im.settings.noChats')}</Typography>
                                    ) : (
                                        <TableContainer>
                                            <Table size="small" sx={{ tableLayout: 'fixed', minWidth: 560, '& td, & th': { borderColor: 'divider' }, '& tbody tr:last-of-type td': { borderBottom: 0 } }}>
                                                <TableHead>
                                                    <TableRow sx={{ '& th': { color: 'text.secondary', fontSize: fontSizes.sm, fontWeight: 500, py: 0.75 } }}>
                                                        <TableCell sx={{ pl: 2, width: '34%' }}>{t('im.settings.chat')}</TableCell>
                                                        <TableCell sx={{ width: '32%' }}>{t('im.settings.boundTo')}</TableCell>
                                                        <TableCell sx={{ width: '18%' }}>{t('im.settings.status')}</TableCell>
                                                        <TableCell align="right" sx={{ pr: 2, width: '16%' }}><Box component="span" sx={{ position: 'absolute', width: 1, height: 1, overflow: 'hidden', clip: 'rect(0 0 0 0)' }}>{t('im.settings.action')}</Box></TableCell>
                                                    </TableRow>
                                                </TableHead>
                                                <TableBody>
                                                    {rows.map((c) => {
                                                        const b = byChat.get(chatKey(c.platform, c.chatId));
                                                        const place = b ? directory.get(b.project_id) : undefined;
                                                        // A thread other than the project's main one is opened on its own.
                                                        const thread = b && place && b.thread_id !== place.main_thread_id ? `?thread=${encodeURIComponent(b.thread_id)}` : '';
                                                        return (
                                                            <TableRow key={chatKey(c.platform, c.chatId)}>
                                                                <TableCell sx={{ pl: 2 }}>
                                                                    <Typography noWrap sx={{ fontWeight: 500, fontSize: fontSizes.base }}>{c.title}</Typography>
                                                                    <Typography noWrap sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{t(`im.chatKind.${c.kind}`)}</Typography>
                                                                </TableCell>
                                                                <TableCell>
                                                                    {b?.project_id ? (
                                                                        <Link component={RouterLink} to={`${paths.project(b.project_id)}${thread}`} underline="hover" color="text.primary" noWrap sx={{ display: 'block', fontSize: fontSizes.base }}>
                                                                            {place?.name ?? t('im.place.elsewhere')}
                                                                        </Link>
                                                                    ) : (
                                                                        <Typography color="text.disabled" sx={{ fontSize: fontSizes.base }}>{t('im.settings.notBound')}</Typography>
                                                                    )}
                                                                </TableCell>
                                                                <TableCell sx={{ whiteSpace: 'nowrap' }}>
                                                                    <Typography component="span" sx={{ fontSize: fontSizes.base, display: 'inline-flex', alignItems: 'center', gap: 0.75, color: 'text.secondary' }}>
                                                                        <Dot color={STATE_COLOR[c.state]} />
                                                                        {t(c.state === 'ok' ? 'im.settings.statusOk' : c.state === 'blocked' ? 'im.settings.statusBlocked' : 'im.settings.statusUnpaired')}
                                                                    </Typography>
                                                                </TableCell>
                                                                <TableCell align="right" sx={{ pr: 2 }}>
                                                                    <Button
                                                                        size="small" variant={b ? 'outlined' : 'contained'} color={b ? 'inherit' : 'primary'}
                                                                        disabled={c.state === 'blocked'}
                                                                        aria-label={b ? t('im.settings.rebindLabel', { chat: c.title }) : undefined}
                                                                        onClick={() => setMoving(c)}
                                                                    >
                                                                        {b ? t('im.settings.rebindAction') : t('im.settings.bindAction')}
                                                                    </Button>
                                                                </TableCell>
                                                            </TableRow>
                                                        );
                                                    })}
                                                </TableBody>
                                            </Table>
                                        </TableContainer>
                                    )}
                                </Surface>
                            );
                        })}
                    </Box>
                )}
            </Box>

            {accessBot && im?.accessDialog?.({ bot: accessBot, onClose: () => { setAccessBot(null); reload(); } })}
            <MoveChatDialog
                chat={moving}
                bound={moving ? byChat.get(chatKey(moving.platform, moving.chatId)) : undefined}
                projects={bindable}
                onClose={() => setMoving(null)}
                onMove={onMove}
            />
        </Box>
    );
}
