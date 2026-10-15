import { useEffect, useState } from 'react';
import { Box, Button, IconButton, InputAdornment, Skeleton, TextField, Typography } from '@mui/material';
import { Link as RouterLink, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Conversation } from '../client';
import { ChatActionsMenu } from '../components/ChatActionsMenu';
import { Add, ChatBubble, MoreHoriz, Search, Star } from '../components/icons';
import { useNotify } from '../hooks/useNotify';
import { timeAgo } from '../utils/format';
import { EmptyState, PageHeader } from '@tingly/ui';

function Row({ chat, onChanged }: { chat: Conversation; onChanged: () => void }) {
    const { t, i18n } = useTranslation('projects');
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    return (
        <Box
            component={RouterLink}
            to={`/chat/${chat.id}`}
            sx={{ display: 'flex', alignItems: 'center', gap: 1.5, px: 2, py: 1.5, textDecoration: 'none', color: 'inherit', borderTop: 1, borderColor: 'divider', '&:hover': { bgcolor: 'action.hover' }, '& .row-menu': { opacity: anchor ? 1 : 0 }, '&:hover .row-menu': { opacity: 1 } }}
        >
            <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography noWrap sx={{ color: 'text.primary', fontWeight: 500, fontSize: '0.875rem' }}>
                    {chat.title || t('chat.untitled')}
                    {chat.starred && <Star sx={{ fontSize: 14, ml: 0.75, verticalAlign: '-2px' }} color="primary" />}
                </Typography>
                <Typography variant="caption" color="text.secondary">{t('chats.lastMessage', { time: timeAgo(chat.updated_at, i18n.language) })}</Typography>
            </Box>
            <IconButton
                className="row-menu"
                size="small"
                aria-label={t('common.more')}
                onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    setAnchor(e.currentTarget);
                }}
            >
                <MoreHoriz fontSize="small" />
            </IconButton>
            <ChatActionsMenu chat={chat} anchorEl={anchor} onClose={() => setAnchor(null)} onChanged={onChanged} onDeleted={onChanged} />
        </Box>
    );
}

export default function ChatsPage() {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const notify = useNotify();
    const [q, setQ] = useState('');
    const [chats, setChats] = useState<Conversation[] | null>(null);
    const [tick, setTick] = useState(0);

    useEffect(() => {
        let alive = true;
        const timer = window.setTimeout(() => {
            unwrap(api.GET('/api/v1/conversations', { params: { query: { q: q || undefined } } }))
                .then((r) => alive && setChats(r.conversations))
                .catch((e: Error) => alive && notify(e.message, 'error'));
        }, q ? 200 : 0);
        return () => {
            alive = false;
            window.clearTimeout(timer);
        };
    }, [q, tick, notify]);

    return (
        <Box sx={{ flex: 1, overflowY: 'auto' }}>
            <Box sx={{ maxWidth: 860, mx: 'auto', px: { xs: 2, md: 4 }, pt: { xs: 7, md: 5 }, pb: 6 }}>
                <PageHeader
                    title={t('chats.title')}
                    actions={<Button variant="contained" startIcon={<Add />} onClick={() => navigate('/chat')}>{t('nav.newChat')}</Button>}
                    sx={{ mb: 3 }}
                />
                <TextField
                    fullWidth
                    autoFocus
                    placeholder={t('chats.search')}
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    slotProps={{ input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }}
                    sx={{ mb: 2, '& .MuiOutlinedInput-root': { bgcolor: 'background.paper' } }}
                />
                {chats === null ? (
                    [0, 1, 2, 3].map((i) => <Skeleton key={i} height={56} />)
                ) : chats.length === 0 ? (
                    <EmptyState
                        compact
                        icon={<ChatBubble sx={{ fontSize: 32 }} />}
                        title={q ? t('chats.noneFound') : t('nav.noChats')}
                        primaryAction={q ? undefined : { label: t('nav.newChat'), onClick: () => navigate('/chat') }}
                    />
                ) : (
                    <>
                        <Typography variant="caption" color="text.secondary">{t('chats.count', { count: chats.length })}</Typography>
                        <Box sx={{ mt: 1, border: 1, borderColor: 'divider', borderRadius: 1.5, overflow: 'hidden', '& > a:first-of-type': { borderTop: 0 } }}>
                            {chats.map((c) => <Row key={c.id} chat={c} onChanged={() => setTick((x) => x + 1)} />)}
                        </Box>
                    </>
                )}
            </Box>
        </Box>
    );
}
