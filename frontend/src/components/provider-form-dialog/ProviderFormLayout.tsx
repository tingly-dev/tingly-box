import {ArrowBack} from '@/components/icons';
import DialogHeader from '@/components/DialogHeader';
import KeyNameField from './KeyNameField';
import {
    Box, Button, Dialog, DialogActions,
    DialogContent, FormControlLabel, Stack, Switch, Typography,
} from '@mui/material';
import {useId, type FormEventHandler, type ReactNode, type Ref} from 'react';
import {useTranslation} from 'react-i18next';

interface LayoutProps {
    open: boolean;
    title?: string;
    description?: string;
    busy: boolean;
    saving: boolean;
    onClose: () => void;
    onSubmit: FormEventHandler<HTMLFormElement>;
    formRef?: Ref<HTMLFormElement>;
    children: ReactNode;
    exportAction?: ReactNode;
    secondaryAction?: ReactNode;
    submitText?: string;
    showCancel?: boolean;
    onBack?: () => void;
}

/** Shared shell keeps every provider editor's sizing, scrolling and actions aligned.
 *
 * Close locks (backdrop, header X, footer Cancel) follow `saving` only — a
 * read-only connection test must never trap the user in the dialog; it is
 * cancelled by the dialog when it closes. Form controls stay locked while
 * `busy` (test running or saving). */
export default function ProviderFormLayout({open, title, description, busy, saving, onClose, onSubmit, formRef,
    children, exportAction, secondaryAction, submitText, showCancel = true, onBack}: LayoutProps) {
    const {t} = useTranslation();
    const titleId = useId();
    return <Dialog open={open} onClose={() => {if (!saving) onClose();}} aria-labelledby={titleId} maxWidth="sm" fullWidth
        slotProps={{paper: {sx: {
            maxHeight: '90dvh', m: {xs: 1.5, sm: 3}, width: {xs: 'calc(100% - 24px)', sm: 'calc(100% - 48px)'},
            display: 'flex', flexDirection: 'column',
        }}}}>
        <DialogHeader title={title || t('providerDialog.editTitle')} titleId={titleId} closeLabel={t('common.close')}
            onClose={onClose} closeDisabled={saving}/>
        <form ref={formRef} onSubmit={onSubmit} noValidate style={{display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0, overflow: 'hidden'}}>
            <DialogContent sx={{px: {xs: 2, sm: 3}, pt: 1, pb: 2.5, overflowY: 'auto', flex: 1}}>
                <Stack component="fieldset" disabled={busy} spacing={2.5} sx={{border: 0, m: 0, p: 0, minWidth: 0}}>
                    {description && <Typography variant="body2" color="text.secondary">{description}</Typography>}
                    {children}
                </Stack>
            </DialogContent>
            <DialogActions disableSpacing sx={{px: {xs: 2, sm: 3}, py: 2, borderTop: 1, borderColor: 'divider', gap: 1.5, flexWrap: 'wrap', flexShrink: 0}}>
                {exportAction && <Box sx={{mr: 'auto'}}>{exportAction}</Box>}
                {onBack && <Button size="small" disabled={busy} startIcon={<ArrowBack fontSize="small"/>} onClick={onBack}>{t('common.back')}</Button>}
                <Box sx={{ml: 'auto', display: 'flex', justifyContent: 'flex-end', gap: 1, flexWrap: 'wrap', '& .MuiButton-root': {whiteSpace: 'nowrap'}}}>
                    {secondaryAction}
                    {showCancel && <Button type="button" color="inherit" size="small" disabled={saving} onClick={onClose}>{t('common.cancel')}</Button>}
                    <Button type="submit" variant="contained" size="small" disabled={busy}>
                        {saving ? t('providerDialog.editUx.saving') : submitText || t('common.saveChanges')}
                    </Button>
                </Box>
            </DialogActions>
        </form>
    </Dialog>;
}

export function ProviderIdentityFields({name, autoName, onNameChange, enabled, onEnabledChange}: {
    name: string; autoName: string; onNameChange: (name: string) => void; enabled: boolean; onEnabledChange: (enabled: boolean) => void;
}) {
    const {t} = useTranslation();
    return <Stack spacing={1.5}>
        <KeyNameField showField onShowField={() => {}} name={name} autoName={autoName} onNameChange={onNameChange}
            helperText={t('providerDialog.editUx.nameHint')}/>
        <Box sx={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 2}}>
            <Box><Typography variant="body2">{t('providerDialog.editUx.status')}</Typography>
                <Typography variant="caption" color="text.secondary">{t(enabled ? 'providerDialog.editUx.activeHint' : 'providerDialog.editUx.inactiveHint')}</Typography></Box>
            <FormControlLabel sx={{m: 0, flexShrink: 0}}
                control={<Switch size="small" checked={enabled} onChange={e => onEnabledChange(e.target.checked)}/>}
                label={t(enabled ? 'providerTable.status.enabled' : 'providerTable.status.disabled')}/>
        </Box>
    </Stack>;
}

export function ProviderFormSection({title, children}: {title: string; children: ReactNode}) {
    return <Stack spacing={1.5}><Typography variant="subtitle2">{title}</Typography>{children}</Stack>;
}
