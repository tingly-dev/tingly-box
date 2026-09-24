import type {RecentFolder} from '@/services/deskApi';
import {Box, Chip, Stack, Typography} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import Composer from './Composer';
import FolderPicker from './FolderPicker';
import PermissionModeSelect from './PermissionModeSelect';

interface NewSessionViewProps {
    initialFolder?: string;
    recentFolders: RecentFolder[];
    permissionModes: string[];
    onCreate: (path: string, prompt: string, permissionMode: string) => Promise<boolean>;
}

// NewSessionView opens straight onto the prompt (ux-principles #2): the
// folder and permission mode sit on the composer as context, prefilled with
// the folder used last, so starting a session is type-and-Enter.
const NewSessionView = ({initialFolder, recentFolders, permissionModes, onCreate}: NewSessionViewProps) => {
    const {t} = useTranslation();
    // null until the user picks or types a folder; until then it follows the
    // requested folder, else the one used last (which may load after mount).
    const [picked, setPicked] = useState<string | null>(null);
    const folder = picked ?? initialFolder ?? recentFolders[0]?.path ?? '';
    const [permissionMode, setPermissionMode] = useState('');

    return (
        <Box sx={{height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', px: 2}}>
            <Box sx={{width: '100%', maxWidth: 720, mb: '10vh'}}>
                <Typography variant="h5" sx={{textAlign: 'center', mb: 3, fontWeight: 500}}>
                    {t('desk.newSessionHeading', {defaultValue: 'What should the agent work on?'})}
                </Typography>
                <Composer
                    autoFocus
                    minRows={3}
                    placeholder={t('desk.promptPlaceholder', {defaultValue: 'Describe a task…'})}
                    canSubmit={folder.trim() !== ''}
                    onSubmit={(prompt) => onCreate(folder.trim(), prompt, permissionMode)}
                    context={(
                        <>
                            <FolderPicker value={folder} onChange={setPicked} recentFolders={recentFolders}/>
                            <PermissionModeSelect value={permissionMode} permissionModes={permissionModes} onChange={setPermissionMode}/>
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
