import { Box, Chip, FormControlLabel, IconButton, List, ListItem, Stack, Switch, Tooltip, Typography } from '@mui/material';
import { DeleteOutline } from '@/components/icons';
import EmptyState from '@/components/EmptyState';
import { getEffectivePolicyState, buildPolicySummary } from './policyPresentation';
import type { DisplayPolicy, PolicyGroup } from './types';

type PolicyListSectionProps = {
    title: string;
    description: string;
    items: DisplayPolicy[];
    kind: 'resource_access' | 'command_execution' | 'content';
    selectedPolicyId: string | null;
    pendingPolicyId: string | null;
    groupsById: Map<string, PolicyGroup>;
    onOpen: (policy: DisplayPolicy) => void;
    onToggle: (policyId: string, enabled: boolean) => void;
    onDelete: (policyId: string) => void;
    onNew: (kind: 'resource_access' | 'command_execution' | 'content') => void;
};

const PolicyListSection = ({
    title,
    description,
    items,
    kind,
    selectedPolicyId,
    pendingPolicyId,
    groupsById,
    onOpen,
    onToggle,
    onDelete,
    onNew,
}: PolicyListSectionProps) => (
    <Box sx={{ mb: 3 }}>
        <Typography
            variant="body2"
            sx={{
                color: "text.secondary",
                mb: 1.5
            }}>
            {description}
        </Typography>
        {items.length === 0 ? (
            <Box sx={{ border: '1px dashed', borderColor: 'divider', borderRadius: 2 }}>
                <EmptyState
                    title={
                        kind === 'resource_access'
                            ? 'No resource access policies yet'
                            : kind === 'command_execution'
                              ? 'No command execution policies yet'
                              : 'No privacy policies yet'
                    }
                    description={
                        kind === 'resource_access'
                            ? 'Start with a guided resource access policy to control reads, writes, deletes, and protected paths.'
                            : kind === 'command_execution'
                              ? 'Start with a guided command execution policy to control dangerous or disallowed commands.'
                              : 'Start with a guided privacy policy to filter model output or tool results.'
                    }
                    compact
                    primaryAction={{
                        label: 'New Policy',
                        onClick: () => onNew(kind),
                    }}
                />
            </Box>
        ) : (
            <List dense sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, py: 0, overflow: 'hidden' }}>
                {items.map((policy) => {
                    const effectiveState = getEffectivePolicyState(policy, groupsById);
                    return (
                        <ListItem
                            key={policy.id}
                            sx={{
                                px: 0,
                                py: 0,
                                borderBottom: '1px solid',
                                borderColor: 'divider',
                                '&:last-child': { borderBottom: 'none' },
                            }}
                        >
                            <Box
                                sx={{
                                    display: 'flex',
                                    alignItems: 'flex-start',
                                    flexDirection: { xs: 'column', lg: 'row' },
                                    gap: 1.5,
                                    width: '100%',
                                    cursor: 'pointer',
                                    px: 2,
                                    py: 1.5,
                                    bgcolor: selectedPolicyId === policy.id ? 'action.selected' : 'transparent',
                                    '&:hover': { bgcolor: 'action.hover' },
                                    opacity: effectiveState.inheritedDisabled ? 0.65 : 1,
                                }}
                                onClick={() => onOpen(policy)}
                            >
                                <Box sx={{ minWidth: { lg: 220 }, flexShrink: 0, minHeight: 0 }}>
                                    <Stack
                                        direction="row"
                                        spacing={0.75}
                                        useFlexGap
                                        sx={{
                                            alignItems: "center",
                                            flexWrap: "wrap"
                                        }}>
                                        <Typography
                                            variant="body2"
                                            sx={{ fontWeight: 600, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '100%' }}
                                        >
                                            {policy.id}
                                        </Typography>
                                        {policy.isBuiltin && (
                                            <Chip
                                                size="small"
                                                label="Built-in"
                                                variant="outlined"
                                                sx={{ height: 20, fontSize: '0.7rem', '& .MuiChip-label': { px: 0.5 } }}
                                            />
                                        )}
                                        {effectiveState.inheritedDisabled && (
                                            <Chip size="small" label="No active group" variant="outlined" />
                                        )}
                                    </Stack>
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            color: "text.secondary",
                                            display: 'block',
                                            mt: 0.5,
                                            minWidth: 0,
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis',
                                            whiteSpace: 'nowrap'
                                        }}>
                                        {policy.name || 'Unnamed policy'}
                                    </Typography>
                                </Box>

                                <Box sx={{ flex: 1, minWidth: 0 }}>
                                    <Typography
                                        variant="body2"
                                        sx={{
                                            color: "text.primary",
                                            whiteSpace: 'nowrap',
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis'
                                        }}>
                                        {buildPolicySummary(policy)}
                                    </Typography>
                                </Box>

                                <Stack
                                    direction={{ xs: 'row', sm: 'row' }}
                                    spacing={1}
                                    sx={{
                                        alignItems: "center",
                                        width: { xs: '100%', lg: 220 },
                                        minWidth: { lg: 220 },
                                        justifyContent: { xs: 'space-between', lg: 'flex-end' },
                                        flexShrink: 0,
                                        alignSelf: 'center'
                                    }}>
                                    <Chip
                                        size="small"
                                        label={
                                            effectiveState.inheritedDisabled
                                                ? 'No active group'
                                                : policy.enabled !== true
                                                  ? 'Disabled'
                                                  : 'Enabled'
                                        }
                                    />
                                    <FormControlLabel
                                        sx={{ ml: 0 }}
                                        onClick={(e) => e.stopPropagation()}
                                        control={
                                            <Switch
                                                size="small"
                                                checked={policy.enabled === true}
                                                disabled={pendingPolicyId === policy.id}
                                                onChange={(e) => onToggle(policy.id, e.target.checked)}
                                            />
                                        }
                                        label="Enabled"
                                    />
                                    <Box sx={{ width: 32, display: 'flex', justifyContent: 'center', flexShrink: 0 }}>
                                        {!policy.isBuiltin && (
                                            <Tooltip title="Delete policy" arrow>
                                                <span>
                                                    <IconButton
                                                        size="small"
                                                        disabled={pendingPolicyId === policy.id}
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            onDelete(policy.id);
                                                        }}
                                                    >
                                                        <DeleteOutline fontSize="small" />
                                                    </IconButton>
                                                </span>
                                            </Tooltip>
                                        )}
                                    </Box>
                                </Stack>
                            </Box>
                        </ListItem>
                    );
                })}
            </List>
        )}
    </Box>
);

export default PolicyListSection;
