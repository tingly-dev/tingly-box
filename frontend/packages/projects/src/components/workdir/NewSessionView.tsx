import { useRef, useState } from 'react';
import { Box, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Attachment, components } from '../../client';
import { Composer } from '../Composer';
import { FolderPicker } from './FolderPicker';
import { ThreadSettingsPickers } from '../ThreadSettingsPickers';
import { EffortPicker } from '../EffortPicker';
import { radius } from '@tingly/ui/theme/radius';
import type { SessionLaunch } from '../../host/ProjectsHost';

interface Props {
    /** The folder the page is scoped to; the last used one when empty. */
    initialFolder?: string;
    /** The known folders' paths. */
    folders: string[];
    permissionModes: string[];
    onCreate: (folder: string, prompt: string, permissionMode: string, launch: SessionLaunch, attachments: Attachment[], effort: components['schemas']['EffortRequest']['effort']) => Promise<boolean>;
    onAddFolder?: () => void;
    /** Stores files for the first message of a session in `folder`. */
    onAttach: (folder: string, files: File[]) => Promise<Attachment[]>;
}

/**
 * Opens straight onto the prompt: the folder is the first picker under the
 * text, so starting a task is type and Enter. The sidebar lists the folders,
 * so there is no second list of them here.
 */
export function NewSessionView({ initialFolder, folders, permissionModes, onCreate, onAddFolder, onAttach }: Props) {
    const { t } = useTranslation('projects');
    const creatingRef = useRef(false);
    const [creating, setCreating] = useState(false);
    const [launch, setLaunch] = useState<SessionLaunch>({ profile: '', model: '' });
    const [picked, setPicked] = useState<string | null>(null);
    const [mode, setMode] = useState('');
    const [effort, setEffort] = useState<components['schemas']['EffortRequest']['effort']>('');
    const [text, setText] = useState('');
    const folder = picked ?? initialFolder ?? folders[0] ?? '';

    return (
        <Box sx={{ height: '100%', overflowY: 'auto', display: 'flex', flexDirection: 'column', px: 2 }}>
            <Box sx={{ width: '100%', maxWidth: 720, mx: 'auto', my: 'auto', py: { xs: 2, md: 5 }, flexShrink: 0 }}>
                <Typography variant="h5" sx={{ textAlign: 'center', mb: 3, fontWeight: 500 }}>{t('session.heading')}</Typography>
                <Box sx={{ mb: 0.75, px: 0.5, minWidth: 0, borderRadius: radius.card, bgcolor: 'action.hover', display: 'flex', alignItems: 'center' }}>
                    <FolderPicker value={folder} onChange={setPicked} folders={folders} onAddFolder={onAddFolder} disabled={creating} />
                </Box>
                <Composer
                    autoFocus
                    keepUntilSent
                    keysHint
                    compactToolbar
                    minRows={3}
                    placeholder={t('session.promptPlaceholder')}
                    canSubmit={folder.trim() !== '' && !creating}
                    text={text}
                    onTextChange={setText}
                    onAccepted={(sent) => setText((prev) => (prev === sent ? '' : prev))}
                    attach={(files) => onAttach(folder.trim(), files)}
                    // Files are stored under the folder's project, so there must be a folder first.
                    attachBlocked={folder.trim() === '' ? t('session.attachNeedsFolder') : undefined}
                    onSend={async (prompt, attachments) => {
                        if (creatingRef.current) return false;
                        creatingRef.current = true;
                        setCreating(true);
                        try {
                            return await onCreate(folder.trim(), prompt, mode, { ...launch }, attachments, effort);
                        } finally {
                            creatingRef.current = false;
                            setCreating(false);
                        }
                    }}
                    leftSlot={(
                        <>
                            <ThreadSettingsPickers folder show="model" value={launch} permissionMode={mode} modes={permissionModes}
                                disabled={creating} onLaunchChange={setLaunch} onPermissionChange={setMode} />
                            <EffortPicker value={effort} disabled={creating}
                                onChange={(value) => setEffort(value as components['schemas']['EffortRequest']['effort'])} />
                            <ThreadSettingsPickers folder show="permission" value={launch} permissionMode={mode} modes={permissionModes}
                                disabled={creating} onLaunchChange={setLaunch} onPermissionChange={setMode} />
                        </>
                    )}
                />
            </Box>
        </Box>
    );
}
