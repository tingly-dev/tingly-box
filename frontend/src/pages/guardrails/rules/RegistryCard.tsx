import { useMemo } from 'react';
import { Alert, Box, Button, Chip, List, ListItem, Stack, Typography } from '@mui/material';
import { ArticleOutlined, Refresh } from '@/components/icons';
import UnifiedCard from '@/components/UnifiedCard';
import type { GuardrailsPolicy, RegistryPolicyEntry } from './types';

type RegistryCardProps = {
    registryURL: string;
    registryLoading: boolean;
    registryLoadError: string | null;
    registryPolicies: RegistryPolicyEntry[];
    policies: GuardrailsPolicy[];
    pendingRegistryInstallIds: Set<string>;
    onRetry: () => void;
    onInstall: (policyId: string) => void;
};

const RegistryCard = ({
    registryURL,
    registryLoading,
    registryLoadError,
    registryPolicies,
    policies,
    pendingRegistryInstallIds,
    onRetry,
    onInstall,
}: RegistryCardProps) => {
    const downloadablePolicies = useMemo(
        () => registryPolicies.slice().sort((a, b) => a.id.localeCompare(b.id)),
        [registryPolicies]
    );

    return (
        <UnifiedCard
            title="Download Management"
            subtitle="Install additional policy fragments from the curated remote registry. After install, enable or disable them from the policy list above."
            size="full"
            rightAction={
                <Stack direction="row" spacing={1}>
                    <Button
                        variant="outlined"
                        size="small"
                        startIcon={<Refresh />}
                        disabled={registryLoading}
                        onClick={onRetry}
                    >
                        {registryLoading ? 'Refreshing…' : 'Retry'}
                    </Button>
                    {registryURL ? (
                        <Button
                            variant="outlined"
                            size="small"
                            component="a"
                            href={registryURL}
                            target="_blank"
                            rel="noreferrer"
                            startIcon={<ArticleOutlined />}
                        >
                            Open Registry
                        </Button>
                    ) : null}
                </Stack>
            }
        >
            <Stack spacing={2}>
                {registryLoadError && <Alert severity="warning">{registryLoadError}</Alert>}
                {!registryLoadError && registryLoading && (
                    <Alert severity="info">Loading remote registry…</Alert>
                )}
                {!registryLoadError && !registryLoading && downloadablePolicies.length === 0 && (
                    <Alert severity="info">No downloadable policies are currently listed in the remote registry.</Alert>
                )}
                {!registryLoadError && !registryLoading && downloadablePolicies.length > 0 && (
                    <List dense sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, py: 0, overflow: 'hidden' }}>
                        {downloadablePolicies.map((policy) => {
                            const installedPolicy = policies.find((item) => item.id === policy.id);
                            const isInstalled = Boolean(installedPolicy);
                            const isEnabled = installedPolicy?.enabled === true;
                            const isInstalling = pendingRegistryInstallIds.has(policy.id);
                            return (
                                <ListItem
                                    key={policy.id}
                                    sx={{
                                        px: 2,
                                        py: 1.5,
                                        borderBottom: '1px solid',
                                        borderColor: 'divider',
                                        '&:last-child': { borderBottom: 'none' },
                                    }}
                                >
                                    <Box sx={{ display: 'flex', alignItems: 'flex-start', flexDirection: { xs: 'column', md: 'row' }, gap: 1.5, width: '100%' }}>
                                        <Box sx={{ minWidth: { md: 240 }, flexShrink: 0 }}>
                                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                {policy.id}
                                            </Typography>
                                            <Typography
                                                variant="caption"
                                                sx={{
                                                    color: "text.secondary",
                                                    display: 'block',
                                                    mt: 0.5
                                                }}>
                                                {policy.name || policy.path}
                                            </Typography>
                                        </Box>
                                        <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center' }}>
                                            <Typography variant="body2" sx={{
                                                color: "text.primary"
                                            }}>
                                                {policy.reason || 'Download this policy fragment from the remote registry and manage enable/disable from the local policy list.'}
                                            </Typography>
                                        </Box>
                                        <Box sx={{ width: { xs: '100%', md: 'auto' }, display: 'flex', justifyContent: { xs: 'flex-start', md: 'flex-end' }, alignItems: 'center', alignSelf: 'center' }}>
                                            {isInstalled && isEnabled && (
                                                <Chip size="small" color="success" label="Enabled" sx={{ mr: 1 }} />
                                            )}
                                            <Button
                                                variant={isInstalled ? 'outlined' : 'contained'}
                                                size="small"
                                                disabled={isInstalled || isInstalling}
                                                onClick={() => onInstall(policy.id)}
                                            >
                                                {isInstalled ? 'Installed' : isInstalling ? 'Installing…' : 'Install'}
                                            </Button>
                                        </Box>
                                    </Box>
                                </ListItem>
                            );
                        })}
                    </List>
                )}
            </Stack>
        </UnifiedCard>
    );
};

export default RegistryCard;
