import { useRef, useState } from 'react';
import { Box, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Attachment } from '../../client';
import { ThreadComposer } from '../ThreadComposer';
import { FolderPicker } from './FolderPicker';
import { ThreadSettingsPickers } from '../ThreadSettingsPickers';
import type { SessionLaunch } from '../../host/ProjectsHost';

interface Props {
    /** The folder the page is scoped to; the last used one when empty. */
    initialFolder?: string;
    /** The known folders' paths. */
    folders: string[];
    permissionModes: string[];
    onCreate: (folder: string, prompt: string, permissionMode: string, launch: SessionLaunch, attachments: Attachment[]) => Promise<boolean>;
    onAddFolder?: () => void;
    /** Stores files for the first message of a session in `folder`. */
    onAttach: (folder: string, files: File[]) => Promise<Attachment[]>;
}

/**
 * Opens straight onto the prompt: the folder is the first picker under the
 * text, so starting a task is type and Enter. The sidebar lists the folders,
 * so there is no second list of them here. The prompt is a draft like any
 * other: what is typed survives leaving the page and switching folders.
 */
export function NewSessionView({ initialFolder, folders, permissionModes, onCreate, onAddFolder, onAttach }: Props) {
    const { t } = useTranslation('projects');
    const creatingRef = useRef(false);
    const [creating, setCreating] = useState(false);
    const [launch, setLaunch] = useState<SessionLaunch>({ profile: '', model: '' });
    const [picked, setPicked] = useState<string | null>(null);
    const [mode, setMode] = useState('');
    const folder = picked ?? initialFolder ?? folders[0] ?? '';

    return (
        <Box sx={{ height: '100%', overflowY: 'auto', display: 'flex', flexDirection: 'column', px: 2 }}>
            <Box sx={{ width: '100%', maxWidth: 720, mx: 'auto', my: 'auto', py: { xs: 2, md: 5 }, flexShrink: 0 }}>
                <Typography variant="h5" sx={{ textAlign: 'center', mb: 3, fontWeight: 500 }}>{t('session.heading')}</Typography>
                <ThreadComposer
                    draftKey="new-session"
                    autoFocus
                    minRows={3}
                    placeholder={t('session.promptPlaceholder')}
                    canSubmit={folder.trim() !== '' && !creating}
                    attach={(files) => onAttach(folder.trim(), files)}
                    // Files are stored under the folder's project, so there must be a folder first.
                    attachBlocked={folder.trim() === '' ? t('session.attachNeedsFolder') : undefined}
                    onSend={async (prompt, attachments) => {
                        if (creatingRef.current) return false;
                        creatingRef.current = true;
                        setCreating(true);
                        try {
                            return await onCreate(folder.trim(), prompt, mode, { ...launch }, attachments);
                        } finally {
                            creatingRef.current = false;
                            setCreating(false);
                        }
                    }}
                    leftSlot={(
                        <>
                            <FolderPicker value={folder} onChange={setPicked} folders={folders} onAddFolder={onAddFolder} disabled={creating} />
                            <ThreadSettingsPickers folder value={launch} permissionMode={mode} modes={permissionModes}
                                disabled={creating} onLaunchChange={setLaunch} onPermissionChange={setMode} />
                        </>
                    )}
                />
            </Box>
        </Box>
    );
}
