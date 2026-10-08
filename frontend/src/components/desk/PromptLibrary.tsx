import {Add, Delete, Edit, PlayArrow} from '@/components/icons';
import {fontMono, fontSizes} from '@/theme/fonts';
import {Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, Stack, TextField, Tooltip, Typography} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import type {DeskPrompt} from './useDeskPrompts';

interface PromptLibraryProps {
    prompts: DeskPrompt[];
    // Run starts a session with this text right away; Fill only puts it in the composer.
    canRun: boolean;
    onRun: (text: string) => void;
    onFill: (text: string) => void;
    onSave: (fields: {id?: string; name: string; text: string}) => boolean;
    onRemove: (id: string) => void;
}

const PromptLibrary = ({prompts, canRun, onRun, onFill, onSave, onRemove}: PromptLibraryProps) => {
    const {t} = useTranslation();
    // `id` undefined = new prompt; a builtin is copied, so its id is dropped.
    const [editing, setEditing] = useState<{id?: string; name: string; text: string} | null>(null);
    const [failed, setFailed] = useState(false);
    const [confirmId, setConfirmId] = useState<string | null>(null);

    return (
        <Box sx={{mt: 3}}>
            <Stack direction="row" sx={{alignItems: 'center', justifyContent: 'space-between', mb: 1}}>
                <Typography variant="subtitle2" sx={{color: 'text.secondary'}}>
                    {t('desk.library.title', {defaultValue: 'Prompt library'})}
                </Typography>
                <Button size="small" startIcon={<Add/>} onClick={() => {setFailed(false); setEditing({name: '', text: ''});}}>
                    {t('desk.library.new', {defaultValue: 'New prompt'})}
                </Button>
            </Stack>
            <Box sx={{display: 'grid', gap: 1.5, gridTemplateColumns: {xs: '1fr', sm: 'repeat(2, 1fr)'}}}>
                {prompts.map((p) => (
                    <Box key={p.id} sx={{border: 1, borderColor: 'divider', borderRadius: 2, p: 1.5, display: 'flex', flexDirection: 'column', gap: 0.75, minWidth: 0}}>
                        <Typography sx={{fontWeight: 500}} noWrap>{p.name}</Typography>
                        <Typography sx={{fontFamily: fontMono, fontSize: fontSizes.sm, color: 'text.secondary', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden'}}>
                            {p.text}
                        </Typography>
                        <Stack direction="row" spacing={0.5} sx={{alignItems: 'center', mt: 'auto'}}>
                            <Tooltip title={canRun ? '' : t('desk.library.needFolder', {defaultValue: 'Choose a project directory first'})}>
                                <span>
                                    <Button size="small" variant="contained" startIcon={<PlayArrow/>} disabled={!canRun} onClick={() => onRun(p.text)}>
                                        {t('desk.library.run', {defaultValue: 'Run'})}
                                    </Button>
                                </span>
                            </Tooltip>
                            <Button size="small" onClick={() => onFill(p.text)}>{t('desk.library.fill', {defaultValue: 'Edit first'})}</Button>
                            <Box sx={{flex: 1}}/>
                            {p.builtin ? (
                                <Button size="small" onClick={() => {setFailed(false); setEditing({name: p.name, text: p.text});}}>
                                    {t('desk.library.copy', {defaultValue: 'Copy to mine'})}
                                </Button>
                            ) : confirmId === p.id ? (
                                <Button size="small" color="error" onClick={() => {onRemove(p.id); setConfirmId(null);}} onBlur={() => setConfirmId(null)} autoFocus>
                                    {t('desk.library.confirmDelete', {defaultValue: 'Delete?'})}
                                </Button>
                            ) : (
                                <>
                                    <IconButton size="small" aria-label={t('common.edit', {defaultValue: 'Edit'})} onClick={() => {setFailed(false); setEditing({id: p.id, name: p.name, text: p.text});}}><Edit fontSize="small"/></IconButton>
                                    <IconButton size="small" aria-label={t('common.delete', {defaultValue: 'Delete'})} onClick={() => setConfirmId(p.id)}><Delete fontSize="small"/></IconButton>
                                </>
                            )}
                        </Stack>
                    </Box>
                ))}
            </Box>
            {editing && (
                <Dialog open onClose={() => setEditing(null)} maxWidth="sm" fullWidth>
                    <form onSubmit={(e) => {
                        e.preventDefault();
                        if (onSave(editing)) setEditing(null); else setFailed(true);
                    }}>
                        <DialogTitle>{editing.id ? t('desk.library.edit', {defaultValue: 'Edit prompt'}) : t('desk.library.new', {defaultValue: 'New prompt'})}</DialogTitle>
                        <DialogContent>
                            <TextField autoFocus fullWidth margin="dense" label={t('desk.library.name', {defaultValue: 'Name'})}
                                value={editing.name} onChange={(e) => setEditing({...editing, name: e.target.value})}/>
                            <TextField fullWidth multiline minRows={4} margin="dense" label={t('desk.library.text', {defaultValue: 'Prompt'})}
                                value={editing.text} onChange={(e) => setEditing({...editing, text: e.target.value})}
                                error={failed} helperText={failed ? t('desk.library.saveFailed', {defaultValue: 'Could not save in this browser. Please try again.'}) : undefined}
                                slotProps={{htmlInput: {style: {fontFamily: fontMono}}}}/>
                        </DialogContent>
                        <DialogActions sx={{px: 3, pb: 2}}>
                            <Button onClick={() => setEditing(null)}>{t('common.cancel', {defaultValue: 'Cancel'})}</Button>
                            <Button type="submit" variant="contained" disabled={!editing.name.trim() || !editing.text.trim()}>{t('common.save', {defaultValue: 'Save'})}</Button>
                        </DialogActions>
                    </form>
                </Dialog>
            )}
        </Box>
    );
};

export default PromptLibrary;
