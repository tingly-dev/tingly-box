import { Box, Card, CardActionArea, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import type { ProjectSummary } from '../../client';
import { useProjectsHost } from '../../host/ProjectsHost';
import { timeAgo } from '../../utils/format';
import { Branch, Pause, Star } from '../icons';
import { ThreadStateMark } from './threadStatus';

export function ProjectCard({ p }: { p: ProjectSummary }) {
    const { t, i18n } = useTranslation('projects');
    const navigate = useNavigate();
    const { paths: routes } = useProjectsHost();
    const c = p.threads;
    const counts = [
        { key: 'waiting', n: c.waiting },
        { key: 'failed', n: c.failed },
        { key: 'ready', n: c.ready },
        { key: 'working', n: c.working },
    ];
    return (
        <Card variant="outlined" sx={{ borderRadius: radius.card, '&:hover': { borderColor: 'text.disabled' } }}>
            <CardActionArea onClick={() => navigate(routes.project(p.id))} sx={{ p: 2.25, height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'stretch', justifyContent: 'flex-start' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="h5" noWrap sx={{ flex: 1 }}>{p.name}</Typography>
                    {p.paused && <Pause sx={{ fontSize: 16 }} color="warning" />}
                    {p.starred && <Star sx={{ fontSize: 16 }} color="primary" />}
                </Box>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, minHeight: 40, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>
                    {p.goal || t('projects.noGoal')}
                </Typography>
                {/* What needs you, in one quiet line: a dot per state that has threads. */}
                <Box sx={{ display: 'flex', columnGap: 1.5, rowGap: 0.5, flexWrap: 'wrap', mt: 1.5, minHeight: 20, fontSize: fontSizes.sm }}>
                    {counts.filter((x) => x.n > 0).map((x) => (
                        <Box key={x.key} component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, color: x.key === 'waiting' ? 'text.primary' : 'text.secondary', fontWeight: x.key === 'waiting' ? 600 : 400 }}>
                            <ThreadStateMark status={x.key} />
                            {x.n} {t(`overview.group.${x.key}`)}
                        </Box>
                    ))}
                </Box>
                {p.repos.length > 0 && (
                    <Typography variant="caption" color="text.secondary" noWrap sx={{ mt: 0.75, display: 'flex', alignItems: 'center', gap: 0.5 }}>
                        <Branch sx={{ fontSize: 13 }} />{p.repos.map((r) => r.path.split('/').pop()).join(' · ')}
                    </Typography>
                )}
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>{t('projects.updated', { time: timeAgo(p.updated_at, i18n.language) })}</Typography>
            </CardActionArea>
        </Card>
    );
}
