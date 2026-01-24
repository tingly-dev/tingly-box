import { CheckCircle } from '@mui/icons-material';
import { Box, Stack, Typography } from '@mui/material';
import React from 'react';
import type { Provider } from '../../types/provider';
import { AuthTypeBadge } from '../AuthTypeBadge';
import { ApiStyleBadge } from '../ApiStyleBadge';

export interface ProviderSidebarProps {
    groupedProviders: Array<{ authType: string; providers: Provider[] }>;
    currentTab: string | undefined;
    selectedProvider?: string;
    onTabChange: (providerUuid: string) => void;
}

export function ProviderSidebar({
    groupedProviders,
    currentTab,
    selectedProvider,
    onTabChange,
}: ProviderSidebarProps) {
    return (
        <Box sx={{
            width: 300,
            borderRight: 1,
            borderColor: 'divider',
            display: 'flex',
            flexDirection: 'column',
            bgcolor: 'background.paper',
        }}>
            {/* Header */}
            <Box sx={{ p: 2, borderBottom: 1, borderColor: 'divider' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                    Credentials ({groupedProviders.flatMap(g => g.providers).length})
                </Typography>
            </Box>

            {/* Vertical Navigation with Auth Type Grouping */}
            <Box
                sx={{
                    flex: 1,
                    overflowY: 'auto',
                    '&::-webkit-scrollbar': {
                        width: 6,
                    },
                    '&::-webkit-scrollbar-thumb': {
                        bgcolor: 'divider',
                        borderRadius: 3,
                    },
                }}
            >
                {groupedProviders.map((group) => {
                    return (
                        <Box key={`group-${group.authType}`}>
                            {/* Auth Type Header */}
                            <Box
                                sx={{
                                    px: 2,
                                    py: 1,
                                    borderBottom: 1,
                                    borderColor: 'divider',
                                    bgcolor: 'action.hover',
                                    position: 'sticky',
                                    top: 0,
                                    zIndex: 1,
                                }}
                            >
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <AuthTypeBadge authType={group.authType} />
                                    <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                                        ({group.providers.length})
                                    </Typography>
                                </Stack>
                            </Box>

                            {/* Provider Items */}
                            {group.providers.map((provider) => {
                                const isProviderSelected = selectedProvider === provider.uuid;
                                const isSelectedTab = currentTab === provider.uuid;

                                return (
                                    <Box
                                        key={provider.uuid}
                                        onClick={() => onTabChange(provider.uuid)}
                                        sx={{
                                            px: 2,
                                            py: 1.5,
                                            cursor: 'pointer',
                                            bgcolor: isSelectedTab ? 'action.selected' : 'transparent',
                                            borderLeft: 3,
                                            borderLeftColor: isSelectedTab ? 'primary.main' : 'transparent',
                                            '&:hover': {
                                                bgcolor: isSelectedTab ? 'action.selected' : 'action.hover',
                                            },
                                            transition: 'all 0.2s',
                                        }}
                                    >
                                        <Stack direction="row" alignItems="center" spacing={1} sx={{ width: '100%', justifyContent: 'space-between' }}>
                                            <Stack direction="row" alignItems="center" spacing={1} sx={{ flex: 1, minWidth: 0 }}>
                                                <Typography
                                                    variant="body2"
                                                    fontWeight={isSelectedTab ? 600 : 400}
                                                    color={isSelectedTab ? 'primary.main' : 'text.primary'}
                                                    noWrap
                                                >
                                                    {provider.name}
                                                </Typography>
                                                {isProviderSelected && (
                                                    <CheckCircle color="primary" sx={{ fontSize: 14, flexShrink: 0 }} />
                                                )}
                                            </Stack>
                                            {provider.api_style && (
                                                <ApiStyleBadge compact={true} apiStyle={provider.api_style} sx={{ flexShrink: 0, width: "100px" }} />
                                            )}
                                        </Stack>
                                    </Box>
                                );
                            })}
                        </Box>
                    );
                })}
            </Box>
        </Box>
    );
}

// Memoize to prevent unnecessary re-renders
export default React.memo(ProviderSidebar);
