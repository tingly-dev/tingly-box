// What the task produced: branch, change summary, push state, and the diff.
// This is the artifact panel — it hands over what the next action needs
// (the branch name, the patch) rather than describing it.
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Box, Button, Chip, CircularProgress, Divider, Stack, Typography} from '@mui/material';
import CodeBlock from '@/components/CodeBlock';
import {Refresh as IconRefresh, Upload as IconUpload} from '@/components/icons';
import {agentApi, type AgentDiff, type AgentSession, type AgentWorkspace} from '@/services/agentApi';

interface Props {
    session: AgentSession;
    workspace?: AgentWorkspace;
    onPushed: (s: AgentSession) => void;
    onError: (msg: string) => void;
}

const ChangesPanel = ({session, workspace, onPushed, onError}: Props) => {
    const {t} = useTranslation();
    const [diff, setDiff] = useState<AgentDiff>();
    const [loading, setLoading] = useState(false);
    const [pushing, setPushing] = useState(false);

    const ready = workspace?.state === 'ready';
    // No branch means the agent works in the user's own directory: there
    // is nothing for tb to push.
    const inPlace = ready && !workspace?.branch;

    const load = useCallback(async () => {
        if (!ready) return;
        setLoading(true);
        const res = await agentApi.diff(session.id);
        setLoading(false);
        if (res.ok) setDiff(res.data);
    }, [session.id, ready]);

    // Refresh whenever the agent settles (idle/done) — that is when the diff changes.
    useEffect(() => {
        load();
    }, [load, session.status, session.artifact?.changed_files]);

    const push = async () => {
        setPushing(true);
        const res = await agentApi.push(session.id);
        setPushing(false);
        if (!res.ok) {
            onError(res.error);
            return;
        }
        onPushed(res.data.session);
    };

    const canPush = ready && !inPlace && session.status !== 'running' && (diff?.changed_files ?? 0) > 0;

    return (
        <Stack spacing={2}>
            <Stack spacing={0.5}>
                <Typography variant="overline" color="text.secondary">{t('tasks.list.branch')}</Typography>
                <Typography variant="body2" sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}>
                    {inPlace ? t('tasks.detail.inPlace') : (workspace?.branch || session.artifact?.branch || '—')}
                </Typography>
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', flexWrap: 'wrap', rowGap: 0.5}}>
                    {session.artifact?.pushed && <Chip size="small" color="success" variant="outlined" label={t('tasks.detail.pushed')} />}
                    {(diff?.changed_files ?? 0) > 0 && (
                        <Typography variant="caption" color="text.secondary">
                            {t('tasks.list.changedFiles', {count: diff?.changed_files ?? 0})}
                        </Typography>
                    )}
                </Stack>
            </Stack>

            <Stack direction={{xs: 'column', sm: 'row'}} spacing={1}>
                {!inPlace && (
                    <Button
                        variant="contained"
                        startIcon={pushing ? <CircularProgress size={16} color="inherit" /> : <IconUpload />}
                        disabled={!canPush || pushing}
                        onClick={push}
                    >
                        {pushing ? t('tasks.detail.pushing') : t('tasks.detail.push')}
                    </Button>
                )}
                <Button variant="outlined" startIcon={<IconRefresh />} disabled={!ready || loading} onClick={load}>
                    {t('tasks.detail.refreshDiff')}
                </Button>
            </Stack>
            {inPlace && (
                <Typography variant="caption" color="text.secondary">{t('tasks.detail.inPlaceNote')}</Typography>
            )}
            {session.artifact?.pushed && !session.artifact?.pr_url && (
                <Typography variant="caption" color="text.secondary">{t('tasks.detail.noPROnBranch')}</Typography>
            )}

            <Divider />

            {!diff || diff.changed_files === 0 ? (
                <Typography variant="body2" color="text.secondary">{t('tasks.detail.noChanges')}</Typography>
            ) : (
                <Stack spacing={1.5}>
                    {diff.stat && (
                        <Box sx={{overflowX: 'auto'}}>
                            <Typography component="pre" variant="caption" sx={{m: 0, fontFamily: 'monospace', whiteSpace: 'pre', color: 'text.secondary'}}>
                                {diff.stat}
                            </Typography>
                        </Box>
                    )}
                    {diff.untracked && diff.untracked.length > 0 && (
                        <Box>
                            <Typography variant="caption" color="text.secondary">{t('tasks.detail.untracked')}</Typography>
                            {diff.untracked.map((f) => (
                                <Typography key={f} variant="caption" sx={{display: 'block', fontFamily: 'monospace', wordBreak: 'break-all'}}>+ {f}</Typography>
                            ))}
                        </Box>
                    )}
                    {diff.patch && (
                        <Box sx={{overflowX: 'auto'}}>
                            <CodeBlock code={diff.patch} language="diff" maxHeight={480} showCopy />
                        </Box>
                    )}
                    {diff.truncated && (
                        <Typography variant="caption" color="text.secondary">{t('tasks.detail.truncated')}</Typography>
                    )}
                </Stack>
            )}
        </Stack>
    );
};

export default ChangesPanel;
