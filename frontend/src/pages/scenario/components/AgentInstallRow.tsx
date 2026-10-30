import { Box, Button, IconButton, Tooltip, Typography } from '@mui/material';
import { useState } from 'react';
import { ContentCopy as CopyIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import { ConfigRow } from '@/components/ConfigRow';
import ChoiceToggle from '@/components/ChoiceToggle';
import { copyableTextStyle } from '@/styles/textStyles';
import type { AgentInstallAction } from './agentSetup';

export interface AgentInstallRowProps {
    command?: string;
    /** The same install from a mirror registry: one row, switched like Local | Docker on the Base URL. */
    mirrorCommand?: string;
    /** Used when installing is not a command (an extension, a download page). */
    actions?: AgentInstallAction[];
    /** The agent's own site: other ways to install than npm. */
    siteUrl?: string;
    onCopy: (text: string, label: string) => void;
}

type Source = 'official' | 'mirror';

/**
 * The connection section's Install row: the command to run, with an
 * official | mirror switch when there is a mirror, or the agent's own install
 * links. Display only: nothing here is ticked or remembered.
 */
export const AgentInstallRow: React.FC<AgentInstallRowProps> = ({ command, mirrorCommand, actions, siteUrl, onCopy }) => {
    const { t } = useTranslation();
    const [source, setSource] = useState<Source>('official');
    const label = t('agentInstall.label');
    if (!command && !actions?.length) return null;

    const shown = source === 'mirror' && mirrorCommand ? mirrorCommand : command;
    const content = command ? (
        <Tooltip title={t('agentInstall.copy')} arrow placement="top">
            <Typography variant="subtitle2" onClick={() => onCopy(shown!, label)} sx={copyableTextStyle}>
                {shown}
            </Typography>
        </Tooltip>
    ) : (
        <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
            {actions!.map((action) => (
                <Button
                    key={`${action.label}-${action.href}`}
                    href={action.href}
                    target={action.external ? '_blank' : undefined}
                    rel={action.external ? 'noopener noreferrer' : undefined}
                    variant={action.variant ?? 'outlined'}
                    size="small"
                >
                    {action.label}
                </Button>
            ))}
        </Box>
    );

    return (
        <Box sx={{ px: 2, py: 0.5 }}>
            <ConfigRow
                activeTab="install"
                onTabChange={() => undefined}
                tabs={[{
                    key: 'install',
                    label,
                    content,
                    actions: command ? (
                        <>
                            <Tooltip title={t('agentInstall.copy')} arrow>
                                <IconButton onClick={() => onCopy(shown!, label)} size="small">
                                    <CopyIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                            {siteUrl && (
                                <Button href={siteUrl} target="_blank" rel="noopener noreferrer" size="small" variant="text" sx={{ flexShrink: 0 }}>
                                    {t('agentInstall.website')}
                                </Button>
                            )}
                            {mirrorCommand && (
                                <ChoiceToggle<Source>
                                    value={source}
                                    onChange={setSource}
                                    ariaLabel={label}
                                    options={[
                                        { value: 'official', label: t('agentInstall.official') },
                                        { value: 'mirror', label: t('agentInstall.mirror'), tooltip: t('agentInstall.mirrorTooltip') },
                                    ]}
                                />
                            )}
                        </>
                    ) : undefined,
                }]}
            />
        </Box>
    );
};
