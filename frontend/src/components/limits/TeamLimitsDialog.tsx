import {Alert, Button, CircularProgress, Dialog, DialogActions, DialogContent, Divider, Stack} from '@mui/material';
import {useId, useState} from 'react';
import {useTranslation} from 'react-i18next';
import DialogHeader from '@/components/DialogHeader';
import {useNotify} from '@/hooks/useNotify';
import LimitsLiveStatus from './LimitsLiveStatus';
import PolicyLimitsFields from './PolicyLimitsFields';
import {useLimitsStatus} from './useLimitsStatus';
import {usePolicyDraft} from './usePolicyDraft';

interface TeamLimitsDialogProps {
    open: boolean;
    onClose: () => void;
    teamId: string;
    teamName: string;
}

/**
 * One entry point for a Team's caps: the whole team together, and what each
 * member gets by default. Both save together; an emptied block clears its policy.
 */
export default function TeamLimitsDialog(props: TeamLimitsDialogProps) {
    const titleId = useId();
    const {t} = useTranslation();
    return (
        <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth aria-labelledby={titleId}>
            <DialogHeader title={t('limits.team.title', {team: props.teamName})} titleId={titleId}
                          closeLabel={t('common.close')} onClose={props.onClose}/>
            <TeamLimitsBody {...props}/>
        </Dialog>
    );
}

function TeamLimitsBody({open, onClose, teamId}: TeamLimitsDialogProps) {
    const {t} = useTranslation();
    const notify = useNotify();
    const whole = usePolicyDraft('team', teamId, open);
    const member = usePolicyDraft('team_member', teamId, open);
    const [saving, setSaving] = useState(false);
    const status = useLimitsStatus(open);
    const wholeEntry = status.find((s) => s.key === `team:${teamId}`);
    const loading = whole.loading || member.loading;

    const handleSave = async () => {
        setSaving(true);
        const [a, b] = await Promise.all([whole.save(), member.save()]);
        setSaving(false);
        if (a?.success && b?.success) {
            notify.success(t('limits.saved'));
            onClose();
        } else {
            notify.error(t('limits.saveFailed'));
        }
    };

    return (
        <>
            <DialogContent>
                <Stack spacing={2.5} sx={{pt: 1}}>
                    <Alert severity="info">{t('limits.team.explain')}</Alert>
                    {loading ? <CircularProgress size={20}/> : (
                        <>
                            <LimitsLiveStatus entry={wholeEntry}/>
                            <PolicyLimitsFields title={t('limits.team.whole')} hint={t('limits.team.wholeHint')}
                                                value={whole.draft} onChange={whole.setDraft}/>
                            <Divider/>
                            <PolicyLimitsFields title={t('limits.team.member')} hint={t('limits.team.memberHint')}
                                                value={member.draft} onChange={member.setDraft}/>
                        </>
                    )}
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose}>{t('limits.cancel')}</Button>
                <Button variant="contained" disabled={loading || saving} onClick={handleSave}>{t('limits.save')}</Button>
            </DialogActions>
        </>
    );
}
