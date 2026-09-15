// What the task changed in the folder: the folder itself, a change summary
// and the patch. Read-only by design — committing and publishing stay with
// the person who owns the folder (.design/managed-agent.md §13).
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Box, Button, CircularProgress, Divider, Stack, Typography} from '@mui/material';
import CodeBlock from '@/components/CodeBlock';
import {Refresh as IconRefresh} from '@/components/icons';
import {agentApi, type AgentDiff, type AgentFolder, type AgentSession} from '@/services/agentApi';

interface Props {
    session: AgentSession;
    folder?: AgentFolder;
}

const ChangesPanel = ({session, folder}: Props) => {
    const {t} = useTranslation();
    const [diff, setDiff] = useState<AgentDiff>();
    const [loading, setLoading] = useState(false);

    const load = useCallback(async () => {
        setLoading(true);
        const res = await agentApi.diff(session.id);
        setLoading(false);
        if (res.ok) setDiff(res.data);
    }, [session.id]);

    // Refresh whenever the agent settles — that is when the diff changes.
    useEffect(() => {
        load();
    }, [load, session.status, session.changed_files]);

    return (
        <Stack spacing={2}>
            <Stack spacing={0.5}>
                <Typography variant="overline" color="text.secondary">{t('tasks.detail.folder')}</Typography>
                <Typography variant="body2" sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}>
                    {folder?.path ?? '—'}
                </Typography>
                {(diff?.changed_files ?? 0) > 0 && (
                    <Typography variant="caption" color="text.secondary">
                        {t('tasks.list.changedFiles', {count: diff?.changed_files ?? 0})}
                    </Typography>
                )}
            </Stack>

            <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                <Button variant="outlined" size="small" startIcon={<IconRefresh />} disabled={loading} onClick={load}>
                    {t('tasks.detail.refreshDiff')}
                </Button>
                {loading && <CircularProgress size={14} />}
            </Stack>
            <Typography variant="caption" color="text.secondary">{t('tasks.detail.inPlaceNote')}</Typography>

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
