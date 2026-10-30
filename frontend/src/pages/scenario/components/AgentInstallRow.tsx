import { Box, Button, IconButton, Tooltip, Typography } from '@mui/material';
import { ContentCopy as CopyIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import { ConfigRow } from '@/components/ConfigRow';
import { copyableTextStyle } from '@/styles/textStyles';
import type { AgentInstallAction } from './AgentSetupCard';

export interface AgentInstallRowProps {
    command?: string;
    mirrorCommand?: string;
    /** Used when installing is not a command (an extension, a download page). */
    actions?: AgentInstallAction[];
    onCopy: (text: string, label: string) => void;
}

const CommandRow: React.FC<{ label: string; command: string; onCopy: AgentInstallRowProps['onCopy'] }> = ({ label, command, onCopy }) => {
    const { t } = useTranslation();
    return (
        <Box sx={{ px: 2, py: 0.5 }}>
            <ConfigRow
                activeTab="install"
                onTabChange={() => undefined}
                tabs={[{
                    key: 'install',
                    label,
                    content: (
                        <Tooltip title={t('agentInstall.copy')} arrow placement="top">
                            <Typography variant="subtitle2" onClick={() => onCopy(command, label)} sx={copyableTextStyle}>
                                {command}
                            </Typography>
                        </Tooltip>
                    ),
                    actions: (
                        <Tooltip title={t('agentInstall.copy')} arrow>
                            <IconButton onClick={() => onCopy(command, label)} size="small">
                                <CopyIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    ),
                }]}
            />
        </Box>
    );
};

/**
 * The connection section's Install row(s): the command to run (and its
 * mirror), or the agent's own install links. Display only: nothing here is
 * ticked or remembered.
 */
export const AgentInstallRow: React.FC<AgentInstallRowProps> = ({ command, mirrorCommand, actions, onCopy }) => {
    const { t } = useTranslation();
    if (command) {
        return (
            <>
                <CommandRow label={t('agentInstall.label')} command={command} onCopy={onCopy} />
                {mirrorCommand && <CommandRow label={t('agentInstall.mirror')} command={mirrorCommand} onCopy={onCopy} />}
            </>
        );
    }
    if (!actions?.length) return null;
    return (
        <Box sx={{ px: 2, py: 0.5 }}>
            <ConfigRow
                activeTab="install"
                onTabChange={() => undefined}
                tabs={[{
                    key: 'install',
                    label: t('agentInstall.label'),
                    content: (
                        <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                            {actions.map((action) => (
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
                    ),
                }]}
            />
        </Box>
    );
};
