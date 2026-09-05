import { GitHub, AppRegistration as NPM, Refresh } from '@/components/icons';
import { Box, Button, Dialog, DialogActions, DialogContent, Divider, Stack, ToggleButton, ToggleButtonGroup, Typography, useTheme } from '@mui/material';
import { alpha } from '@mui/material/styles';
import { fontMono } from '@/theme/fonts';
import React, { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useVersion } from '@/contexts/VersionContext';
import { CopyIconButton } from '@/components/CopyIconButton';
import { Paper } from '@mui/material';

interface UpdatePanelDialogProps {
    open: boolean;
    onClose: () => void;
}

/**
 * UpdatePanelDialog Component
 *
 * Comprehensive dialog for version checking and update instructions.
 * Displays current vs latest version, manual check button, and multiple
 * installation methods with copy-to-clipboard functionality.
 */
export const UpdatePanelDialog: React.FC<UpdatePanelDialogProps> = ({ open, onClose }) => {
    const { t } = useTranslation();
    const theme = useTheme();
    const { currentVersion, latestVersion, checking, releaseURL, checkForUpdates, hasUpdate } = useVersion();

    const [selectedMethodId, setSelectedMethodId] = useState<string>('npx');

    const displayCurrentVersion = (currentVersion || 'Unknown').split('+')[0];
    const displayLatestVersion = (latestVersion || currentVersion || 'Unknown').split('+')[0];

    // Use backend's has_update for accurate version comparison
    const hasVersionUpdate = hasUpdate && latestVersion && currentVersion;

    // Determine which version to use for commands
    // If update is available, use latest version
    // If up to date, use latest version (not current_version which might be "dev" in dev mode)
    // Only fallback to currentVersion if latestVersion is not available
    const versionForCommand = latestVersion || currentVersion;

    // Update methods with commands - always use specific version.
    // Selected via the channel toggle below; only one method is expanded at a
    // time so adding channels doesn't grow the dialog.
    const updateMethods = [
        {
            id: 'npx',
            title: t('update.methods.npx.title'),
            description: t('update.methods.npx.description'),
            commands: [versionForCommand ? `npx tingly-box@${versionForCommand}` : 'npx tingly-box@latest'],
            icon: <NPM />,
        },
        {
            id: 'npm',
            title: t('update.methods.npm.title'),
            description: t('update.methods.npm.description'),
            // Two commands on purpose (no `&&`, which older PowerShell lacks):
            // pasting the pair runs them sequentially in any shell.
            commands: [
                versionForCommand ? `npm install -g tingly-box@${versionForCommand}` : 'npm install -g tingly-box@latest',
                'tingly-box restart',
            ],
            icon: <NPM />,
        },
        {
            id: 'docker',
            title: t('update.methods.docker.title'),
            description: t('update.methods.docker.description'),
            commands: [versionForCommand ? `docker pull ghcr.io/tingly-dev/tingly-box:v${versionForCommand}` : 'docker pull ghcr.io/tingly-dev/tingly-box:latest'],
            icon: <GitHub />,
        },
    ] as const;

    const selectedMethod = updateMethods.find((m) => m.id === selectedMethodId) ?? updateMethods[0];

    const handleCheckForUpdates = useCallback(() => {
        checkForUpdates(true);
    }, [checkForUpdates]);

    const getStatusColor = () => {
        if (checking) return 'info';
        if (hasVersionUpdate) return 'warning';
        return 'success';
    };

    const statusColor = getStatusColor();
    const statusPalette = theme.palette[statusColor];

    const getStatusIcon = () => {
        if (checking) {
            return <Refresh sx={{ fontSize: 32, color: statusPalette.main, animation: 'spin 1s linear infinite' }} />;
        }
        if (hasVersionUpdate) {
            return <GitHub sx={{ fontSize: 32, color: statusPalette.main }} />;
        }
        return <Refresh sx={{ fontSize: 32, color: statusPalette.main }} />;
    };

    const getStatusTitle = () => {
        if (checking) {
            return t('update.checking');
        }
        if (hasVersionUpdate) {
            return t('update.updateAvailable');
        }
        return t('update.upToDate');
    };

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="sm"
            fullWidth
            slotProps={{
                paper: {
                    sx: {
                        borderRadius: 2,
                        overflow: 'hidden',
                        border: '1px solid',
                        borderColor: 'divider',
                    },
                }
            }}
        >
            {/* Header — soft tinted surface derived from the status palette color */}
            <Box
                sx={{
                    bgcolor: alpha(statusPalette.main, theme.palette.mode === 'dark' ? 0.16 : 0.10),
                    borderBottom: '1px solid',
                    borderColor: alpha(statusPalette.main, 0.24),
                    px: 3,
                    py: 2.5,
                    textAlign: 'center',
                }}
            >
                <Box
                    sx={{
                        width: 56,
                        height: 56,
                        borderRadius: '50%',
                        bgcolor: alpha(statusPalette.main, theme.palette.mode === 'dark' ? 0.24 : 0.16),
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        mx: 'auto',
                        mb: 1.5,
                    }}
                >
                    {getStatusIcon()}
                </Box>
                <Typography variant="h6" sx={{ color: 'text.primary', fontWeight: 600, mb: 0.5 }}>
                    {getStatusTitle()}
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5 }}>
                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                        {hasVersionUpdate ? (
                            t('update.versionComparison', {
                                latest: displayLatestVersion,
                                current: displayCurrentVersion,
                            })
                        ) : (
                            t('update.currentVersion', { version: displayCurrentVersion })
                        )}
                    </Typography>
                    <CopyIconButton
                        value={displayCurrentVersion}
                        label={t('update.copy')}
                        copiedLabel={t('update.copied')}
                        tooltipPlacement="top"
                        tooltipArrow
                        iconSize={16}
                    />
                </Box>
            </Box>
            <DialogContent sx={{ p: 0 }}>
                <Stack spacing={0} divider={<Divider />}>
                    {/* Manual Check Section */}
                    <Box sx={{ p: 2.5 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1.5, color: 'text.primary' }}>
                            {t('update.checkUpdates')}
                        </Typography>
                        <Button
                            variant={hasVersionUpdate ? 'contained' : 'outlined'}
                            color={hasVersionUpdate ? 'warning' : 'primary'}
                            onClick={handleCheckForUpdates}
                            disabled={checking}
                            startIcon={checking ? <Refresh sx={{ fontSize: 18, animation: 'spin 1s linear infinite' }} /> : <Refresh />}
                            fullWidth
                            sx={{ height: 48 }}
                        >
                            {checking ? t('update.checking') : t('update.check')}
                        </Button>
                    </Box>

                    {/* Update Methods Section */}
                    <Box sx={{ p: 2.5 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1.5, color: 'text.primary' }}>
                            {t('update.updateMethods')}
                        </Typography>

                        {/* Channel selector: literal channel names (npx / npm / …) so the
                            toggle reads as the real-world install method, with the
                            translated title + description shown for the selected one. */}
                        <ToggleButtonGroup
                            value={selectedMethod.id}
                            exclusive
                            onChange={(_, value) => value && setSelectedMethodId(value)}
                            size="small"
                            fullWidth
                            sx={{ mb: 1.5 }}
                        >
                            {updateMethods.map((method) => (
                                <ToggleButton key={method.id} value={method.id} sx={{ textTransform: 'none', fontFamily: fontMono }}>
                                    {method.id}
                                </ToggleButton>
                            ))}
                        </ToggleButtonGroup>

                        <Typography
                            variant="body2"
                            sx={{
                                fontWeight: 500,
                                mb: 0.5,
                                color: 'text.primary',
                                display: 'flex',
                                alignItems: 'center',
                                gap: 0.5,
                            }}
                        >
                            {selectedMethod.icon}
                            {selectedMethod.title}
                        </Typography>
                        <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mb: 1 }}>
                            {selectedMethod.description}
                        </Typography>
                        <Paper
                            variant="outlined"
                            sx={{
                                p: 2,
                                bgcolor: 'background.paper',
                                border: '1px solid',
                                borderColor: 'divider',
                                position: 'relative',
                            }}
                        >
                            {selectedMethod.commands.map((command) => (
                                <Typography
                                    key={command}
                                    variant="body2"
                                    sx={{
                                        fontFamily: fontMono,
                                        color: 'text.primary',
                                        fontSize: '0.875rem',
                                        pr: 5,
                                        wordBreak: 'break-all',
                                    }}
                                >
                                    $ {command}
                                </Typography>
                            ))}
                            <CopyIconButton
                                value={selectedMethod.commands.join('\n')}
                                label={t('update.copy')}
                                copiedLabel={t('update.copied')}
                                tooltipPlacement="top"
                                tooltipArrow
                                iconSize={18}
                                sx={{
                                    position: 'absolute',
                                    right: 8,
                                    top: '50%',
                                    transform: 'translateY(-50%)',
                                    '&:hover': { color: 'primary.main', bgcolor: 'action.hover' },
                                }}
                            />
                        </Paper>
                    </Box>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 2, bgcolor: 'action.hover', justifyContent: 'space-between' }}>
                <Button
                    onClick={() => window.open(releaseURL || 'https://github.com/tingly-dev/tingly-box/releases', '_blank')}
                    startIcon={<GitHub />}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': {
                            bgcolor: 'action.selected',
                        },
                    }}
                >
                    {t('update.releaseNotes')}
                </Button>
                <Button
                    onClick={onClose}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': {
                            bgcolor: 'action.selected',
                        },
                    }}
                >
                    {t('common.close')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default UpdatePanelDialog;
