import { useState } from 'react';
import { Box, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Workdir } from '../../client';
import { Composer } from '../Composer';
import { FolderPicker } from './FolderPicker';
import { PermissionModeSelect } from './PermissionModeSelect';

interface Props {
    /** The folder the page is scoped to; the last used one when empty. */
    initialFolder?: string;
    folders: Workdir[];
    permissionModes: string[];
    onCreate: (folder: string, prompt: string, permissionMode: string) => Promise<boolean>;
    onAddFolder?: () => void;
    /** A host's own pickers (profile, model), after the folder and permission mode. */
    context?: React.ReactNode;
}

/**
 * Opens straight onto the prompt: the folder is the first picker under the
 * text, so starting a task is type and Enter. The sidebar lists the folders,
 * so there is no second list of them here.
 */
export function NewSessionView({ initialFolder, folders, permissionModes, onCreate, onAddFolder, context }: Props) {
    const { t } = useTranslation('projects');
    const [picked, setPicked] = useState<string | null>(null);
    const [mode, setMode] = useState('');
    const [text, setText] = useState('');
    const folder = picked ?? initialFolder ?? folders[0]?.path ?? '';

    return (
        <Box sx={{ height: '100%', overflowY: 'auto', display: 'flex', flexDirection: 'column', px: 2 }}>
            <Box sx={{ width: '100%', maxWidth: 720, mx: 'auto', my: 'auto', py: { xs: 2, md: 5 }, flexShrink: 0 }}>
                <Typography variant="h5" sx={{ textAlign: 'center', mb: 3, fontWeight: 500 }}>{t('session.heading')}</Typography>
                <Composer
                    autoFocus
                    keepUntilSent
                    keysHint
                    minRows={3}
                    placeholder={t('session.promptPlaceholder')}
                    canSubmit={folder.trim() !== ''}
                    text={text}
                    onTextChange={setText}
                    onAccepted={(sent) => setText((prev) => (prev === sent ? '' : prev))}
                    onSend={(prompt) => onCreate(folder.trim(), prompt, mode)}
                    leftSlot={(
                        <>
                            <FolderPicker value={folder} onChange={setPicked} folders={folders} onAddFolder={onAddFolder} />
                            {context}
                            <PermissionModeSelect value={mode} modes={permissionModes} onChange={setMode} />
                        </>
                    )}
                />
            </Box>
        </Box>
    );
}
