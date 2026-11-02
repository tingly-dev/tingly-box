import { useRef } from 'react';
import { Alert, Box, Button, ButtonGroup, Chip, CircularProgress, Divider, FormControl, InputLabel, IconButton, List, ListItemButton, ListItemText, MenuItem, Select, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import PageHeader from '@/components/PageHeader';
import PageLayout from '@/components/PageLayout';
import Surface from '@/components/Surface';
import EmptyState from '@/components/EmptyState';
import { Refresh, FolderOpen, Description } from '@/components/icons';
import { useLocalSessions } from './localSessions/useLocalSessions';
import type { SessionBlock, SessionEvent } from '@/services/localSessionsApi';

function size(bytes: number): string {
    const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    let index = 0;
    while (bytes >= 1024 && index < units.length - 1) { bytes /= 1024; index++; }
    return `${index ? bytes.toFixed(1) : bytes} ${units[index]}`;
}
function timestamp(value?: string) { const date = value ? new Date(value) : undefined; return date && !Number.isNaN(date.getTime()) ? date.toLocaleString() : ''; }

function ContentBlock({ block }: { block: SessionBlock }) {
    const { t } = useTranslation();
    const text = <Box component="pre" sx={{ m: 0, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontSize: '0.875rem', fontFamily: block.kind === 'text' ? 'inherit' : 'monospace', lineHeight: 1.65 }}>{block.text}</Box>;
    const truncated = block.truncated && <Typography variant="caption" color="text.secondary">{t('localSessions.truncated')}</Typography>;
    if (block.kind === 'text' || block.kind === 'summary') return <Box>{text}{truncated}</Box>;
    if (block.kind === 'attachment') return <Typography variant="body2" color="text.secondary">{t('localSessions.attachment', { type: block.label })}</Typography>;
    if (block.kind === 'notice') {
        const label = block.label === 'additional_blocks_omitted' ? t('localSessions.moreBlocks')
            : block.label === 'event_too_large' ? t('localSessions.largeEvent')
            : block.label === 'compact_boundary' ? t('localSessions.compacted') : block.label || t('localSessions.system');
        return <Typography variant="caption" color="text.secondary">{label}</Typography>;
    }
    return <Box component="details" sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1 }}>
        <Box component="summary" sx={{ cursor: 'pointer', fontSize: '0.8rem', color: 'text.secondary' }}>
            {block.kind === 'thinking' ? t('localSessions.thinking') : t('localSessions.tool')}{block.label ? ` · ${block.label}` : ''}
        </Box>
        <Box sx={{ mt: 1 }}>{text}{truncated}</Box>
    </Box>;
}
function EventCard({ event }: { event: SessionEvent }) {
    const { t } = useTranslation();
    const toolOnly = event.blocks.every(b => b.kind === 'tool_use' || b.kind === 'tool_result');
    return <Box sx={{ py: 2, px: { xs: 1.5, md: 2.5 }, borderBottom: 1, borderColor: 'divider' }}>
        <Stack direction="row" sx={{ gap: 1, alignItems: 'center', flexWrap: 'wrap', mb: 1 }}>
            <Typography variant="subtitle2">{toolOnly ? t('localSessions.tool') : event.role === 'user' ? t('localSessions.user') : event.role === 'assistant' ? 'Claude' : t('localSessions.system')}</Typography>
            {event.sidechain && <Chip size="small" label={t('localSessions.branch')} />}
            <Typography variant="caption" color="text.secondary" sx={{ ml: 'auto' }}>{timestamp(event.timestamp)}</Typography>
        </Stack>
        <Stack spacing={1}>{event.blocks.map((block, i) => <ContentBlock key={i} block={block} />)}</Stack>
    </Box>;
}
export default function LocalSessionsPage() {
    const { t } = useTranslation();
    const state = useLocalSessions();
    const previewContainer = useRef<HTMLDivElement>(null);
    const selected = state.sessions?.entries.find(e => e.id === state.session);
    const busy = Object.values(state.loading).some(Boolean);
    const step = (action: () => void) => { action(); if (previewContainer.current) previewContainer.current.scrollTop = 0; };
    return <PageLayout loading={false}>
        <Stack spacing={2}>
            <PageHeader title={t('localSessions.title')} subtitle={t('localSessions.subtitle')}
                actions={<Button startIcon={<Refresh />} disabled={busy} onClick={state.refresh}>{t('localSessions.refresh')}</Button>} />
            {state.error && <Alert severity="warning">{state.error} {t('localSessions.retry')}</Alert>}
            {state.loading.status && <Box sx={{ p: 4, textAlign: 'center' }}><CircularProgress size={24} /></Box>}
            {state.status && !state.allowed && <Alert severity="info">{!state.status.local_binding ? t('localSessions.localBinding') : state.status.local_access ? t('localSessions.disabled') : t('localSessions.localOnly')}</Alert>}
            {state.allowed && <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '320px minmax(0, 1fr)' }, gap: 2, alignItems: 'start' }}>
                <Surface sx={{ p: 2, minWidth: 0 }}>
                    <Stack spacing={1.5}>
                        <FormControl fullWidth size="small" disabled={state.loading.projects}>
                            <InputLabel id="session-project-label">{t('localSessions.project')}</InputLabel>
                            <Select labelId="session-project-label" label={t('localSessions.project')} value={state.project} onChange={e => state.setProject(e.target.value)}>
                                {(state.projects?.entries || []).map(project => <MenuItem key={project.id} value={project.id}>{project.title || project.name}</MenuItem>)}
                            </Select>
                        </FormControl>
                        {(state.hasPrevProjects || state.projects?.next_cursor) && <Stack direction="row" sx={{ justifyContent: 'space-between' }}>
                            <Button size="small" disabled={!state.hasPrevProjects || busy} onClick={state.prevProjects}>{t('localSessions.previousProjects')}</Button>
                            <Button size="small" disabled={!state.projects?.next_cursor || busy} onClick={state.nextProjects}>{t('localSessions.nextProjects')}</Button>
                        </Stack>}
                        {state.loading.projects && <CircularProgress size={20} />}
                        {state.projects?.entries.length === 0 && <EmptyState compact title={t('localSessions.noProjects')} icon={<FolderOpen />} />}
                        <Stack direction="row" sx={{ justifyContent: 'space-between' }}><Typography variant="subtitle2">{t('localSessions.sessions')}</Typography><Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}><Typography variant="caption" color="text.secondary">{state.sessions?.total ?? ''}</Typography><IconButton size="small" disabled={busy} aria-label={t('localSessions.refreshLists')} title={t('localSessions.refreshLists')} onClick={state.refreshLists}><Refresh fontSize="small" /></IconButton></Box></Stack>
                        <Divider />
                        {state.loading.sessions && <Box sx={{ p: 2 }}><CircularProgress size={20} /></Box>}
                        <List disablePadding sx={{ maxHeight: { xs: 260, md: '58vh' }, overflow: 'auto' }}>
                            {(state.sessions?.entries || []).map(entry => <ListItemButton key={entry.id} selected={entry.id === state.session} onClick={() => state.setSession(entry.id)} sx={{ borderRadius: 1, mb: 0.5 }}>
                                <ListItemText primary={entry.title || entry.name.replace(/\.jsonl$/, '')} secondary={`${timestamp(entry.modified)} · ${size(entry.size)}`}
                                    slotProps={{ primary: { sx: { fontSize: '0.85rem', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden', overflowWrap: 'anywhere' } }, secondary: { sx: { fontSize: '0.72rem' } } }} />
                            </ListItemButton>)}
                        </List>
                        {state.sessions?.entries.length === 0 && <Typography variant="body2" color="text.secondary">{t('localSessions.noSessions')}</Typography>}
                        <Stack direction="row" sx={{ justifyContent: 'space-between' }}>
                            <Button size="small" disabled={!state.hasPrevSessions || busy} onClick={state.prevSessions}>{t('localSessions.newer')}</Button>
                            <Button size="small" disabled={!state.sessions?.next_cursor || busy} onClick={state.nextSessions}>{t('localSessions.older')}</Button>
                        </Stack>
                    </Stack>
                </Surface>
                <Surface padding={0} sx={{ minWidth: 0, overflow: 'hidden' }}>
                    <Stack direction="row" sx={{ gap: 1, alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', p: 2, borderBottom: 1, borderColor: 'divider' }}>
                        <Box sx={{ minWidth: 0, flex: 1 }}><Typography variant="subtitle2" noWrap>{selected?.title || selected?.name || t('localSessions.selectSession')}</Typography><Typography variant="caption" color="text.secondary">{t('localSessions.readOnly')}</Typography></Box>
                        <ButtonGroup size="small" disabled={!state.session || busy}>
                            <Button variant={state.direction === 'backward' ? 'contained' : 'outlined'} onClick={() => step(() => state.jumpTo('backward'))}>{t('localSessions.recent')}</Button>
                            <Button variant={state.direction === 'forward' ? 'contained' : 'outlined'} onClick={() => step(() => state.jumpTo('forward'))}>{t('localSessions.beginning')}</Button>
                        </ButtonGroup>
                    </Stack>
                    <Box ref={previewContainer} sx={{ maxHeight: '65vh', minHeight: 280, overflow: 'auto' }}>
                        {state.loading.preview && <Box sx={{ p: 5, textAlign: 'center' }}><CircularProgress size={24} /></Box>}
                        {!state.loading.preview && !state.session && <EmptyState compact title={t('localSessions.selectSession')} icon={<Description />} />}
                        {state.preview?.events.map((event, i) => <EventCard key={`${event.id}:${i}`} event={event} />)}
                        {state.preview?.events.length === 0 && <EmptyState compact title={t('localSessions.noEvents')} description={t('localSessions.windowHint')} />}
                    </Box>
                    {state.preview && <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider' }}>
                        <Typography variant="caption" color="text.secondary">{t('localSessions.windowInfo', { count: state.preview.events.length, scanned: size(state.preview.scanned_bytes), size: size(state.preview.file_size) })}</Typography>
                        {state.preview.skipped_records > 0 && <Typography variant="caption" sx={{ display: 'block' }} color="text.secondary">{t('localSessions.skipped', { count: state.preview.skipped_records })}</Typography>}
                        <Stack direction="row" sx={{ justifyContent: 'space-between', mt: 1 }}>
                            <Button size="small" disabled={!state.hasPrevPreview || busy} onClick={() => step(state.prevPreview)}>{t(state.direction === 'backward' ? 'localSessions.newer' : 'localSessions.earlier')}</Button>
                            <Button size="small" disabled={!state.preview.next_cursor || busy} onClick={() => step(state.nextPreview)}>{t(state.direction === 'backward' ? 'localSessions.earlier' : 'localSessions.later')}</Button>
                        </Stack>
                    </Box>}
                </Surface>
            </Box>}
        </Stack>
    </PageLayout>;
}
