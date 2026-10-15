import type {RecentFolder} from '@/services/deskApi';
import {ArrowBack} from '@/components/icons';
import {Box, Button, Typography} from '@mui/material';
import {Composer} from '@tingly/projects';
import {useTranslation} from 'react-i18next';
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
// working folder is the first picker under the text, beside the launch
// settings, so starting a task is type-and-Enter. The sidebar already lists
// the folders, so there is no second list of them here.
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
                <Composer
                    autoFocus
                    keepUntilSent
                    keysHint
                    minRows={3}
                    placeholder={t('desk.promptPlaceholder', {defaultValue: 'Describe a task…'})}
                    canSubmit={folder.trim() !== ''}
                    text={form.prompt ?? ''}
                    onTextChange={(text) => update('prompt', text)}
                    onAccepted={(submitted) => setForm((previous) => previous.prompt === submitted ? {...previous, prompt: ''} : previous)}
                    onSend={(prompt) => onCreate(folder.trim(), prompt, permissionMode, profile, model)}
                    leftSlot={(
                        <>
                            <FolderPicker value={folder} onChange={setPicked} recentFolders={recentFolders} onAddFolder={onAddProject}/>
                            <ProfileSelect value={profile} onChange={pickProfile}/>
                            <ModelSelect profile={profile} value={model} onChange={(value) => update('model', value)}/>
                            <PermissionModeSelect value={permissionMode} permissionModes={permissionModes} onChange={(value) => update('permissionMode', value)}/>
                        </>
                    )}
                />
            </Box>
        </Box>
    );
};

export default NewSessionView;
