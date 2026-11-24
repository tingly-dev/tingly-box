import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Box, IconButton, MenuItem, Select, Tooltip, Typography } from '@mui/material';
import ConfigRow from '@/components/ConfigRow';
import { ArrowForward, InfoOutlined } from '@/components/icons';
import { useProfileContext } from '@/contexts/ProfileContext';
import { useNotify } from '@/hooks/useNotify';
import { useMCPProfiles } from '@/features/mcp/profileStore';
import { MCP_DEFAULT_PROFILE_ID } from '@/features/mcp/types';
import { api } from '@/services/api';

const NONE = '';

// Which MCP profile a Claude Code profile's launches connect to. The user
// chooses; nothing is connected by default. Claude Code gets the MCP profile
// as an ordinary MCP server (--mcp-config) and its own permissions decide each
// call, so there is nothing else to set here. Hidden while MCP is off, since
// the endpoint would refuse the connection.
export default function ClaudeCodeMCPRow({ scenario, profileId }: { scenario: string; profileId: string }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();
    const { getProfiles, refresh } = useProfileContext();
    const mcpProfiles = useMCPProfiles();
    const [enabled, setEnabled] = useState(false);
    const [saving, setSaving] = useState(false);
    const current = getProfiles(scenario).find((p) => p.id === profileId)?.mcp_profile || NONE;

    useEffect(() => {
        let mounted = true;
        void api
            .getMCPConfig()
            .then((response) => mounted && setEnabled(Boolean(response?.success && response.enabled)))
            .catch(() => undefined);
        return () => {
            mounted = false;
        };
    }, []);

    if (!enabled) return null;
    const label = (id: string, name?: string) =>
        id === MCP_DEFAULT_PROFILE_ID
            ? t('mcp.workspace.defaultProfileName', {
                  defaultValue: 'MCP Integration',
              })
            : name || id;
    const missing = current !== NONE && !mcpProfiles.some((p) => p.id === current);

    const choose = async (next: string) => {
        setSaving(true);
        const result = await api.setProfileMCPProfile(scenario, profileId, next);
        setSaving(false);
        if (result?.success) {
            refresh();
        } else {
            notify.error(result?.error || 'Failed to save');
        }
    };

    return (
        <Box sx={{ px: 2, pt: 1 }}>
            <ConfigRow
                tabs={[
                    {
                        key: 'mcp',
                        label: t('claudeCode.profile.mcpLabel'),
                        content: (
                            <Select
                                size="small"
                                variant="standard"
                                disableUnderline
                                value={current}
                                displayEmpty
                                disabled={saving}
                                onChange={(event) => void choose(event.target.value)}
                                inputProps={{ 'aria-label': t('claudeCode.profile.mcpLabel') }}
                                data-testid="cc-profile-mcp"
                                renderValue={(value) => (
                                    <Typography variant="subtitle2" color={missing ? 'warning.main' : undefined}>
                                        {value === NONE
                                            ? t('claudeCode.profile.mcpNone')
                                            : missing
                                              ? `${value} · ${t('claudeCode.profile.mcpMissing')}`
                                              : label(value, mcpProfiles.find((p) => p.id === value)?.name)}
                                    </Typography>
                                )}
                            >
                                <MenuItem value={NONE}>{t('claudeCode.profile.mcpNone')}</MenuItem>
                                {mcpProfiles.map((p) => (
                                    <MenuItem key={p.id} value={p.id}>
                                        {label(p.id, p.name)}
                                    </MenuItem>
                                ))}
                                {missing && (
                                    <MenuItem value={current} disabled>
                                        {current}
                                    </MenuItem>
                                )}
                            </Select>
                        ),
                        actions: (
                            <>
                                <Tooltip title={t('claudeCode.profile.mcpHint')} arrow>
                                    <IconButton size="small" aria-label={t('claudeCode.profile.mcpHint')}>
                                        <InfoOutlined fontSize="small" sx={{ color: 'text.secondary' }} />
                                    </IconButton>
                                </Tooltip>
                                {current !== NONE && !missing && (
                                    <Tooltip title={t('claudeCode.profile.mcpOpen')}>
                                        <IconButton
                                            size="small"
                                            aria-label={t('claudeCode.profile.mcpOpen')}
                                            onClick={() => navigate(`/mcp/profile/${encodeURIComponent(current)}`)}
                                        >
                                            <ArrowForward fontSize="small" />
                                        </IconButton>
                                    </Tooltip>
                                )}
                            </>
                        ),
                    },
                ]}
                activeTab="mcp"
                onTabChange={() => {}}
            />
        </Box>
    );
}
