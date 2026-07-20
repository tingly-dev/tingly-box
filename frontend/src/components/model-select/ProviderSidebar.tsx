import { CheckCircle, Search } from '@/components/icons';
import { Box, Stack, Typography, TextField, InputAdornment } from '@mui/material';
import React, { useState, useMemo } from 'react';
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
    const [providerSearchTerm, setProviderSearchTerm] = useState('');

    // Filter providers based on search term
    const filteredGroupedProviders = useMemo(() => {
        if (!providerSearchTerm.trim()) {
            return groupedProviders;
        }

        const searchLower = providerSearchTerm.toLowerCase();
        return groupedProviders
            .map(group => ({
                authType: group.authType,
                providers: group.providers.filter(provider =>
                    provider.name.toLowerCase().includes(searchLower) ||
                    provider.uuid.toLowerCase().includes(searchLower)
                ),
            }))
            .filter(group => group.providers.length > 0);
    }, [groupedProviders, providerSearchTerm]);
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
                <TextField
                    size="small"
                    placeholder="Search credentials..."
                    value={providerSearchTerm}
                    onChange={(e) => setProviderSearchTerm(e.target.value)}
                    slotProps={{
                        input: {
                            startAdornment: (
                                <InputAdornment position="start">
                                    <Search />
                                </InputAdornment>
                            ),
                        },
                    }}
                    sx={{ width: 200 }}
                />
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
                {filteredGroupedProviders.map((group) => {
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
                                <Stack direction="row" spacing={1} sx={{
                                    alignItems: "center"
                                }}>
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
                                        <Stack
                                            direction="row"
                                            spacing={1}
                                            sx={{
                                                alignItems: "center",
                                                width: '100%',
                                                justifyContent: 'space-between'
                                            }}>
                                            <Stack
                                                direction="row"
                                                spacing={1}
                                                sx={{
                                                    alignItems: "center",
                                                    flex: 1,
                                                    minWidth: 0
                                                }}>
                                                <Typography
                                                    variant="body2"
                                                    color={isSelectedTab ? 'primary.main' : 'text.primary'}
                                                    noWrap
                                                    sx={{
                                                        fontWeight: isSelectedTab ? 600 : 400
                                                    }}
                                                >
                                                    {provider.name}
                                                </Typography>
                                                {isProviderSelected && (
                                                    <CheckCircle color="primary" sx={{ fontSize: 14, flexShrink: 0 }} />
                                                )}
                                            </Stack>
                                            {provider.api_base_openai && provider.api_base_anthropic ? (
                                                <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0 }}>
                                                    <ApiStyleBadge compact={true} apiStyle="openai" />
                                                    <ApiStyleBadge compact={true} apiStyle="anthropic" />
                                                </Stack>
                                            ) : (
                                                provider.api_style && (
                                                    <ApiStyleBadge compact={true} apiStyle={provider.api_style} sx={{ flexShrink: 0, width: "100px" }} />
                                                )
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
