import { Box, Card, CardActionArea, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import type { ProjectSummary } from '../../client';
import { useProjectsHost } from '../../host/ProjectsHost';
import { folderName, timeAgo } from '../../utils/format';
import { Folder } from '../icons';
import { ThreadStateMark } from '../project/threadStatus';

/** A working folder as a card: its name, where it is, and which sessions need you. */
export function WorkdirCard({ f }: { f: ProjectSummary }) {
    const { t, i18n } = useTranslation('projects');
    const navigate = useNavigate();
    const { paths } = useProjectsHost();
    const path = f.path ?? '';
    const counts = [
        { key: 'waiting', n: f.threads.waiting },
        { key: 'failed', n: f.threads.failed },
        { key: 'ready', n: f.threads.ready },
        { key: 'working', n: f.threads.working },
    ];
    return (
        <Card variant="outlined" sx={{ borderRadius: radius.card, '&:hover': { borderColor: 'text.disabled' } }}>
            <CardActionArea onClick={() => navigate(paths.folder(path))} sx={{ p: 2.25, height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'stretch', justifyContent: 'flex-start' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Folder sx={{ fontSize: 18 }} color="action" />
                    <Typography variant="h5" noWrap sx={{ flex: 1 }}>{f.name || folderName(path)}</Typography>
                </Box>
                <Typography variant="caption" color="text.secondary" noWrap title={path} sx={{ mt: 0.5, fontFamily: fontMono }}>{path}</Typography>
                <Box sx={{ display: 'flex', columnGap: 1.5, rowGap: 0.5, flexWrap: 'wrap', mt: 1.5, minHeight: 20, fontSize: fontSizes.sm }}>
                    {counts.filter((x) => x.n > 0).map((x) => (
                        <Box key={x.key} component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, color: x.key === 'waiting' ? 'text.primary' : 'text.secondary', fontWeight: x.key === 'waiting' ? 600 : 400 }}>
                            <ThreadStateMark status={x.key} />
                            {x.n} {t(`overview.group.${x.key}`)}
                        </Box>
                    ))}
                </Box>
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>{t('projects.updated', { time: timeAgo(f.updated_at, i18n.language) })}</Typography>
            </CardActionArea>
        </Card>
    );
}
