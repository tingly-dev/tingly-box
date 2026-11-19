import {Alert, Button, CircularProgress, Dialog, DialogActions, DialogContent, Stack} from '@mui/material';
import {useId, useState} from 'react';
import {useTranslation} from 'react-i18next';
import DialogHeader from '@tingly/ui/components/DialogHeader';
import {useNotify} from '@/hooks/useNotify';
import {isEmptyLimits} from './LimitsEditor';
import LimitsLiveStatus from './LimitsLiveStatus';
import PolicyLimitsFields, {EMPTY_POLICY_DRAFT} from './PolicyLimitsFields';
import {useLimitsStatus} from './useLimitsStatus';
import {usePolicyDraft, type PolicyKind} from './usePolicyDraft';

interface PolicyLimitsDialogProps {
    open: boolean;
    onClose: () => void;
    kind: PolicyKind;
    refId?: string;
    title: string;
    hint?: string;
    /** Called after a successful save/clear so callers can refresh badges. */
    onSaved?: () => void;
}

/** Edit the limits of one person (a sharing key's user) or one model. */
export default function PolicyLimitsDialog(props: PolicyLimitsDialogProps) {
    const titleId = useId();
    const {t} = useTranslation();
    return (
        <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth aria-labelledby={titleId}>
            <DialogHeader title={props.title} titleId={titleId} closeLabel={t('common.close')} onClose={props.onClose}/>
            <PolicyLimitsDialogBody {...props}/>
        </Dialog>
    );
}

function PolicyLimitsDialogBody({open, onClose, kind, refId, hint, onSaved}: PolicyLimitsDialogProps) {
    const {t} = useTranslation();
    const notify = useNotify();
    const {draft, setDraft, loading, save} = usePolicyDraft(kind, refId, open);
    const [saving, setSaving] = useState(false);
    const status = useLimitsStatus(open && !!refId);
    const entry = status.find((s) => s.key === `${kind}:${refId}`);
    const inherited = entry?.inherited_from === 'team_member' && isEmptyLimits(draft.limits);

    const handleSave = async () => {
        setSaving(true);
        const res = await save();
        setSaving(false);
        if (res?.success) {
            notify.success(t('limits.saved'));
            onSaved?.();
            onClose();
        } else {
            notify.error(res?.error?.message || res?.error || t('limits.saveFailed'));
        }
    };

    return (
        <>
            <DialogContent>
                <Stack spacing={2} sx={{pt: 1}}>
                    {inherited && <Alert severity="info">{t('limits.inheritedFromTeam')}</Alert>}
                    <LimitsLiveStatus entry={entry}/>
                    {loading
                        ? <CircularProgress size={20}/>
                        : <PolicyLimitsFields hint={hint} value={draft} onChange={setDraft}/>}
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button color="inherit" disabled={loading || isEmptyLimits(draft.limits)} onClick={() => setDraft(EMPTY_POLICY_DRAFT)}>
                    {t('limits.clear')}
                </Button>
                <Button onClick={onClose}>{t('limits.cancel')}</Button>
                <Button variant="contained" disabled={loading || saving} onClick={handleSave}>
                    {t('limits.save')}
                </Button>
            </DialogActions>
        </>
    );
}
