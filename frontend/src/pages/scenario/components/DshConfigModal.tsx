import { Box, CircularProgress, DialogActions, DialogContent, IconButton, Tooltip, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { RestartAlt } from '@/components/icons';
import DshQuickConfig, { type DshPrefs, defaultDshPrefs, mergeSavedDshPrefs } from './DshQuickConfig';
import { ConfigModalShell, QuickApplyActions } from './config/ConfigModalShell';
import { ManualFileSection } from './config/ManualFileSection';
import { writeFileScripts } from './config/writeFileScripts';
import { useAppliedPrefs } from './config/useAppliedPrefs';
import { useDebouncedPreview } from './config/useDebouncedPreview';
import { api } from '@/services/api';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';

interface DshConfigModalProps {
    open: boolean;
    onClose: () => void;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    // Shared page-level toast, used for apply success/error so feedback is
    // consistent with the rest of the scenario pages.
    showNotification?: (message: string, severity: 'success' | 'error' | 'info' | 'warning') => void;
}

type MainTab = 'quick' | 'manual';

const DSH_API_KEY_ENV = 'TINGLY_BOX_API_KEY';

const DshConfigModal: React.FC<DshConfigModalProps> = ({
    open,
    onClose,
    copyToClipboard,
    showNotification,
}) => {
    const { t } = useTranslation();
    // Fallback for the credentials.yaml preview while the preview request is
    // in flight.
    const { token } = useScenarioPageModal();
    const [mainTab, setMainTab] = React.useState<MainTab>('quick');
    const [prefs, setPrefs] = React.useState<DshPrefs>(() => defaultDshPrefs());
    const [settingsYaml, setSettingsYaml] = React.useState<string>('# Loading...');
    const [credentialsYaml, setCredentialsYaml] = React.useState<string>(`${DSH_API_KEY_ENV}: "${token}"\n`);
    const [previewModels, setPreviewModels] = React.useState<string[]>([]);
    const [isApplying, setIsApplying] = React.useState(false);

    // On open, restore the prefs previously applied to
    // $DSH_HOME/settings.yaml; first-time users fall back to defaults.
    // True while the readback is in flight, so the Quick tab can show a
    // spinner instead of flashing the defaults before the saved values land.
    const isConfigLoading = useAppliedPrefs({
        open,
        fetch: () => api.getAppliedDshConfig(),
        applySaved: (result) => setPrefs(mergeSavedDshPrefs(result.preferences || {})),
        applyDefaults: () => setPrefs(defaultDshPrefs()),
    });

    // Re-render the server-authoritative YAML whenever prefs change while the
    // modal is open.
    useDebouncedPreview({
        open,
        deps: [prefs, token],
        fetch: async () => {
            const resp = await api.getDshConfigPreview(prefs as Record<string, string>);
            if (resp?.success) {
                setSettingsYaml(resp.settingsYaml || '');
                setCredentialsYaml(resp.credentialsYaml || `${DSH_API_KEY_ENV}: "${token}"\n`);
                setPreviewModels(resp.models || []);
            }
        },
    });

    // Here-doc write scripts for the $DSH_HOME files; only the file name
    // differs between the settings.yaml / .credentials.yaml steps.
    const dshWriteScripts = (fileVar: string, filename: string, content: string) => writeFileScripts({
        dirVar: '$dshHome',
        fileVar,
        dirSetupWindows: `$dshHome = if ($env:DSH_HOME) { $env:DSH_HOME } else { Join-Path $HOME ".dsh" }`,
        fileSetupWindows: `${fileVar} = Join-Path $dshHome "${filename}"`,
        dirSetupUnix: `DSH_HOME="\${DSH_HOME:-$HOME/.dsh}"\nmkdir -p "$DSH_HOME"`,
        fileUnix: `"$DSH_HOME/${filename}"`,
        content,
    });

    const settingsScripts = dshWriteScripts('$settingsPath', 'settings.yaml', settingsYaml);
    const credsScripts = dshWriteScripts('$credsPath', '.credentials.yaml', credentialsYaml);

    const handleApplyConfiguration = async () => {
        setIsApplying(true);
        try {
            const response = await api.applyDshConfig(prefs as Record<string, string>);
            if (response?.success) {
                showNotification?.(t('dshConfig.applySuccess'), 'success');
            } else {
                showNotification?.(response?.message || t('dshConfig.applyFailed'), 'error');
            }
        } catch (err: any) {
            showNotification?.(err?.message || t('dshConfig.applyFailed'), 'error');
        } finally {
            setIsApplying(false);
        }
    };

    return (
        <ConfigModalShell
            open={open}
            onClose={onClose}
            title={t('dshConfig.title')}
            subtitle={t('dshConfig.subtitle')}
            headerAction={mainTab === 'quick' && (
                <Tooltip title={t('dshConfig.resetTooltip')} arrow>
                    <IconButton
                        size="small"
                        onClick={() => setPrefs(defaultDshPrefs())}
                        sx={{ position: 'absolute', top: 12, right: 12 }}
                    >
                        <RestartAlt fontSize="small" />
                    </IconButton>
                </Tooltip>
            )}
            tabs={{
                value: mainTab,
                onChange: (value) => setMainTab(value as MainTab),
                items: [
                    { value: 'quick', label: t('dshConfig.tabQuick') },
                    { value: 'manual', label: t('dshConfig.tabManual') },
                ],
            }}
        >
            <DialogContent sx={{ p: 3 }}>
                {mainTab === 'quick' && isConfigLoading && (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                        <CircularProgress size={28} />
                    </Box>
                )}

                {mainTab === 'quick' && !isConfigLoading && (
                    <DshQuickConfig prefs={prefs} setPrefs={setPrefs} />
                )}

                {mainTab === 'manual' && (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                        <ManualFileSection
                            heading="Step 1 · Create or update `$DSH_HOME/settings.yaml`"
                            copyToClipboard={copyToClipboard}
                            tabs={[
                                {
                                    label: 'YAML',
                                    value: 'yaml',
                                    code: settingsYaml,
                                    language: 'yaml',
                                    filename: 'Create or update $DSH_HOME/settings.yaml',
                                    copyLabel: 'settings.yaml',
                                    maxHeight: 220,
                                    minHeight: 180,
                                },
                                {
                                    label: 'Windows',
                                    value: 'windows',
                                    code: settingsScripts.windows,
                                    language: 'js',
                                    filename: 'PowerShell script to setup settings.yaml',
                                    copyLabel: 'Windows settings script',
                                    maxHeight: 260,
                                    minHeight: 220,
                                },
                                {
                                    label: 'Linux/macOS',
                                    value: 'unix',
                                    code: settingsScripts.unix,
                                    language: 'js',
                                    filename: 'Bash script to setup settings.yaml',
                                    copyLabel: 'Unix settings script',
                                    maxHeight: 260,
                                    minHeight: 220,
                                },
                            ]}
                        />

                        <ManualFileSection
                            heading="Step 2 · Create or update `$DSH_HOME/.credentials.yaml`"
                            description={`Set \`${DSH_API_KEY_ENV}\` in \`$DSH_HOME/.credentials.yaml\` to the API key generated by Tingly Box. If the file already exists, update the existing value.`}
                            copyToClipboard={copyToClipboard}
                            tabs={[
                                {
                                    label: 'YAML',
                                    value: 'yaml',
                                    code: credentialsYaml,
                                    language: 'yaml',
                                    filename: 'Create or update $DSH_HOME/.credentials.yaml',
                                    copyLabel: '.credentials.yaml',
                                    maxHeight: 140,
                                    minHeight: 100,
                                },
                                {
                                    label: 'Windows',
                                    value: 'windows',
                                    code: credsScripts.windows,
                                    language: 'js',
                                    filename: 'PowerShell script to setup .credentials.yaml',
                                    copyLabel: 'Windows credentials script',
                                    maxHeight: 220,
                                    minHeight: 180,
                                },
                                {
                                    label: 'Linux/macOS',
                                    value: 'unix',
                                    code: credsScripts.unix,
                                    language: 'js',
                                    filename: 'Bash script to setup .credentials.yaml',
                                    copyLabel: 'Unix credentials script',
                                    maxHeight: 220,
                                    minHeight: 180,
                                },
                            ]}
                        />

                        {previewModels.length > 0 && (
                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                {t('dshConfig.modelsPreviewNote', { models: previewModels.join(', ') })}
                            </Typography>
                        )}
                    </Box>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <QuickApplyActions
                    onClose={onClose}
                    onApply={handleApplyConfiguration}
                    applying={isApplying}
                />
            </DialogActions>
        </ConfigModalShell>
    );
};

export default DshConfigModal;
