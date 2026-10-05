import type {RecentFolder} from '@/services/deskApi';
import {ArrowBack} from '@/components/icons';
import {Box, Button, Chip, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import Composer from './Composer';
import FolderPicker from './FolderPicker';
import ModelSelect from './ModelSelect';
import PermissionModeSelect from './PermissionModeSelect';
import ProfileSelect from './ProfileSelect';
import {useDeskDrafts} from './useDeskDrafts';

interface NewSessionViewProps {
    initialFolder?: string;
    recentFolders: RecentFolder[];
    permissionModes: string[];
    onCreate: (path: string, prompt: string, permissionMode: string, profile: string, model: string) => Promise<boolean>;
    onAddProject?: () => void;
    onBack?: () => void;
}

// NewSessionView opens straight onto the prompt (ux-principles #2): the
// project directory has its own visible field above the composer; launch
// settings stay with the prompt, so starting a task is type-and-Enter.
const NewSessionView = ({initialFolder, recentFolders, permissionModes, onCreate, onAddProject, onBack}: NewSessionViewProps) => {
    const {t} = useTranslation();
    // null until the user picks or types a folder; until then it follows the
    // requested folder, else the one used last (which may load after mount).
    const [form, setForm] = useDeskDrafts(`desk.newDraft:${initialFolder ?? ''}`);
    const update = (field: string, value: string) => setForm((prev) => ({...prev, [field]: value}));
    const setPicked = (value: string) => update('folder', value);
    const folder = form.folder ?? initialFolder ?? recentFolders[0]?.path ?? '';
    const permissionMode = form.permissionMode ?? '';
    const profile = form.profile ?? '';
    // A tier belongs to the profile it was picked under.
    const model = form.model ?? '';
    const pickProfile = (p: string) => {
        setForm((prev) => ({...prev, profile: p, model: ''}));
    };

    return (
        <Box sx={{height: '100%', overflowY: 'auto', display: 'flex', flexDirection: 'column', px: 2}}>
            {onBack && <Button onClick={onBack} startIcon={<ArrowBack/>} sx={{alignSelf: 'flex-start', mt: 1}}>
                {t('common.back', {defaultValue: 'Back'})}
            </Button>}
            <Box sx={{width: '100%', maxWidth: 720, mx: 'auto', mt: 'auto', mb: 'auto', py: {xs: 2, md: 5}, flexShrink: 0}}>
                <Typography variant="h5" sx={{textAlign: 'center', mb: 3, fontWeight: 500}}>
                    {t('desk.newSessionHeading', {defaultValue: 'What should the agent work on?'})}
                </Typography>
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', mb: 1.5}}>
                    <FolderPicker value={folder} onChange={setPicked} recentFolders={recentFolders}/>
                    {onAddProject && <Button size="small" onClick={onAddProject} sx={{flexShrink: 0}}>{t('desk.addProject', {defaultValue: 'Add project'})}</Button>}
                </Stack>
                <Typography variant="caption" sx={{display: 'block', mb: 1.5, color: 'text.secondary'}}>
                    {t('desk.taskDirectoryHint', {defaultValue: 'Choose a project directory, then describe the task below.'})}
                </Typography>
                <Composer
                    autoFocus
                    minRows={3}
                    placeholder={t('desk.promptPlaceholder', {defaultValue: 'Describe a task…'})}
                    canSubmit={folder.trim() !== ''}
                    text={form.prompt ?? ''}
                    onTextChange={(text) => update('prompt', text)}
                    onAccepted={(submitted) => setForm((previous) => previous.prompt === submitted ? {...previous, prompt: ''} : previous)}
                    onSubmit={(prompt) => onCreate(folder.trim(), prompt, permissionMode, profile, model)}
                    context={(
                        <>
                            <ProfileSelect value={profile} onChange={pickProfile}/>
                            <ModelSelect profile={profile} value={model} onChange={(value) => update('model', value)}/>
                            <PermissionModeSelect value={permissionMode} permissionModes={permissionModes} onChange={(value) => update('permissionMode', value)}/>
                        </>
                    )}
                />
                {recentFolders.length > 1 && (
                    <Stack direction="row" spacing={0.75} sx={{mt: 1.5, flexWrap: 'wrap', rowGap: 0.75, justifyContent: 'center'}}>
                        {recentFolders.slice(0, 6).map((f) => (
                            <Chip
                                key={f.path}
                                size="small"
                                label={f.name}
                                title={f.path}
                                variant={folder === f.path ? 'filled' : 'outlined'}
                                onClick={() => setPicked(f.path)}
                            />
                        ))}
                    </Stack>
                )}
            </Box>
        </Box>
    );
};

export default NewSessionView;
