import { useTranslation } from 'react-i18next';
import { useProjectsHost, type SessionLaunch } from '../host/ProjectsHost';
import { ModelPicker } from './ModelPicker';
import { PermissionModeSelect } from './workdir/PermissionModeSelect';

interface Props {
    value: SessionLaunch;
    permissionMode: string;
    modes: string[];
    onLaunchChange: (next: SessionLaunch) => void;
    onPermissionChange: (mode: string) => void;
    disabled?: boolean;
    folder?: boolean;
    defaultModel?: string;
    defaultPermissionMode?: string;
    show?: 'all' | 'model' | 'permission';
}

/** The same independent choices for a new folder thread and every open thread. */
export function ThreadSettingsPickers({ value, permissionMode, modes, onLaunchChange, onPermissionChange, disabled, folder = false, defaultModel, defaultPermissionMode = 'acceptEdits', show = 'all' }: Props) {
    const { t } = useTranslation('projects');
    const { session } = useProjectsHost();
    const modelDefault = folder ? t('threadSettings.defaultModel')
        : `${t('threadSettings.projectDefault')} · ${defaultModel || t('threadSettings.defaultModel')}`;
    const permissionDefault = `${t(folder ? 'threadSettings.defaultPermissions' : 'threadSettings.projectDefault')} · ${t(`session.mode.${defaultPermissionMode}`, { defaultValue: defaultPermissionMode })}`;
    return (
        <>
            {show !== 'permission' && (folder && session?.launchPickers ? session.launchPickers({ value, onChange: onLaunchChange, disabled }) : (
                <ModelPicker source="projects" value={value.model} defaultLabel={modelDefault} defaultDisplayLabel={defaultModel || t('threadSettings.defaultModel')}
                    inheritedModel={defaultModel} disabled={disabled} onChange={(model) => onLaunchChange({ ...value, model })} />
            ))}
            {show !== 'model' && <PermissionModeSelect value={permissionMode} modes={modes} disabled={disabled} defaultLabel={permissionDefault}
                defaultDisplayLabel={t(`session.mode.${defaultPermissionMode}`, { defaultValue: defaultPermissionMode })}
                label={t('threadSettings.permissions')} onChange={onPermissionChange} />}
        </>
    );
}
