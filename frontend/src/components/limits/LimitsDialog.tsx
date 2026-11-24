import {Button, Dialog, DialogActions, DialogContent, Stack, Typography} from '@mui/material';
import {useId, useState} from 'react';
import {useTranslation} from 'react-i18next';
import DialogHeader from '@tingly/ui/components/DialogHeader';
import LimitsEditor, {compactLimits, isEmptyLimits, type Limits} from './LimitsEditor';
import LimitsLiveStatus from './LimitsLiveStatus';
import {useLimitsStatus} from './useLimitsStatus';

interface LimitsDialogProps {
    open: boolean;
    onClose: () => void;
    value?: Limits;
    /** Called with the compacted limits; `{}` clears. */
    onSave: (next: Limits) => void;
    /** Status-entry key to show live (e.g. svc:<rule>/<provider>/<model>). */
    statusKey?: string;
    title?: string;
    hint?: string;
}

/** Edit one service's share of an account. */
export default function LimitsDialog(props: LimitsDialogProps) {
    const {open, onClose} = props;
    const titleId = useId();
    const {t} = useTranslation();
    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth aria-labelledby={titleId}>
            <DialogHeader title={props.title ?? t('limits.titleService')} titleId={titleId} closeLabel={t('common.close')} onClose={onClose}/>
            {/* The body mounts only while open, so the draft re-initialises from `value` on every open. */}
            <LimitsDialogBody {...props}/>
        </Dialog>
    );
}

function LimitsDialogBody({open, onClose, value, onSave, statusKey, hint}: LimitsDialogProps) {
    const {t} = useTranslation();
    const [draft, setDraft] = useState<Limits>(value ?? {});
    const status = useLimitsStatus(open && !!statusKey);
    const entry = status.find((s) => s.key === statusKey);
    return (
        <>
            <DialogContent>
                <Stack spacing={2} sx={{pt: 1}}>
                    <Typography variant="body2" color="text.secondary">{hint ?? t('limits.hintService')}</Typography>
                    <LimitsLiveStatus entry={entry}/>
                    <LimitsEditor value={draft} onChange={setDraft}/>
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button color="inherit" disabled={isEmptyLimits(draft)} onClick={() => setDraft({})}>
                    {t('limits.clear')}
                </Button>
                <Button onClick={onClose}>{t('limits.cancel')}</Button>
                <Button variant="contained" onClick={() => { onSave(compactLimits(draft)); onClose(); }}>
                    {t('limits.save')}
                </Button>
            </DialogActions>
        </>
    );
}
