import {
    Add as AddIcon,
    Warning as WarningIcon,
    ExpandMore as ExpandMoreIcon,
} from '@mui/icons-material';
import {
    Box,
    Button,
    Card,
    CardContent,
    Chip,
    Stack,
    Tooltip,
    Typography,
    IconButton,
    Collapse,
} from '@mui/material';
import { alpha, styled } from '@mui/material/styles';
import React from 'react';
import type { Provider } from '../types/provider';
import {
    SmartOpNode,
    ActionAddNode,
    SmartDefaultNode,
    ArrowNode,
    ModelNode,
    NodeContainer,
    ProviderNode, MODEL_NODE_STYLES,
    getRouteGraphActiveColor,
} from '@/components/nodes';
import type { ConfigRecord } from './RoutingGraphTypes.ts';

// Use same style constants as RuleGraph for consistency
const RULE_GRAPH_STYLES = {
    node: {
        width: 320,
        height: 120,
        heightCompact: 60,
        padding: 10,
    },
    spacing: {
        xs: 4,
        sm: 8,
        md: 12,
        lg: 16,
        xl: 16,
    },
    header: {
        paddingX: 16,
        paddingY: 6,   // spacing(0.75) - reduced from 8 for smaller height
    },
    graphContainer: {
        paddingX: 16,
        paddingY: 8,
        marginX: 16,
        marginY: 6,
    },
    graph: {
        stackSpacing: 0,
        modelGap: 8,
        labelMargin: 4,
        rowGap: 16,
        iconGap: 4,
        wrapperGap: 8,
    },
} as const;

const { header, graphContainer, graph } = RULE_GRAPH_STYLES;

interface SmartRoutingGraphProps {
    record: ConfigRecord;
    providers: Provider[];
    active: boolean;
    onAddSmartRule: () => void;
    onEditSmartRule: (ruleUuid: string) => void;
    onDeleteSmartRule: (ruleUuid: string) => void;
    onAddServiceToSmartRule: (ruleIndex: number) => void;
    onDeleteServiceFromSmartRule: (ruleUuid: string, serviceUuid: string) => void;
    onAddDefaultProvider?: () => void;
    onDeleteDefaultProvider?: (providerUuid: string) => void;
    onProviderNodeClick?: (providerUuid: string) => void;
    // Per-service priority edit on the default-providers row. Smart mode's
    // default providers behave identically to normal-mode services (same
    // Rule.Services list, same load-balancing tactic), so the same
    // priority UI applies.
    onProviderPriorityChange?: (providerUuid: string, priority: number) => void;
    // Additional props matching RoutingGraph
    saving?: boolean;
    collapsible?: boolean;
    expanded?: boolean;
    onToggleExpanded?: () => void;
    extraActions?: React.ReactNode;
    extensionsCard?: React.ReactNode;
    onUpdateRecord?: (field: keyof ConfigRecord, value: any) => void;
    // Routing mode switch
    onSwitchRoutingMode?: () => void;
}

// Styled Card matching RuleGraph style
const StyledCard = styled(Card, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    transition: 'border-color 0.16s ease, background-color 0.16s ease, opacity 0.16s ease, box-shadow 0.18s ease',
    opacity: active ? 1 : 0.6,
    filter: active ? 'none' : 'grayscale(0.3)',
    border: active ? '1px solid' : '2px dashed',
    borderColor: active ? theme.palette.divider : theme.palette.text.disabled,
    boxShadow: 'none',
    margin: "3px",
    position: 'relative',
    ...(active ? {} : {
        '&::before': {
            content: '""',
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundImage: 'repeating-linear-gradient(45deg, transparent, transparent 10px, rgba(0,0,0,0.03) 10px, rgba(0,0,0,0.03) 20px)',
            pointerEvents: 'none',
            borderRadius: theme.shape.borderRadius,
        },
    }),
    '&:hover': {
        borderColor: active ? alpha(getRouteGraphActiveColor(theme), 0.55) : theme.palette.text.disabled,
        boxShadow: active
            ? `0 0 0 3px ${alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.12 : 0.10)}`
            : 'none',
    },
}));

const SummarySection = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'collapsible',
})<{ collapsible?: boolean }>(({ collapsible }) => ({
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: `${header.paddingY}px ${header.paddingX}px`,
    cursor: collapsible ? 'pointer' : 'default',
    ...(collapsible && {
        '&:hover': {
            backgroundColor: 'action.hover',
        },
    }),
}));

const GraphContainer = styled(Box)(({ theme }) => ({
    padding: `${graphContainer.paddingY}px ${graphContainer.paddingX}px`,
    backgroundColor: 'grey.50',
    borderRadius: theme.shape.borderRadius,
    margin: `${graphContainer.marginY}px ${graphContainer.marginX}px 0`,
}));

const GraphRow = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-start',
    gap: graph.rowGap,
    marginBottom: theme.spacing(0.5),
}));

const SmartRoutingGraph: React.FC<SmartRoutingGraphProps> = ({
                                                                 record,
                                                                 providers,
                                                                 active,
                                                                 onAddSmartRule,
                                                                 onEditSmartRule,
                                                                 onDeleteSmartRule,
                                                                 onAddServiceToSmartRule,
                                                                 onDeleteServiceFromSmartRule,
                                                                 onAddDefaultProvider,
                                                                 onDeleteDefaultProvider,
                                                                 onProviderNodeClick,
                                                                 onProviderPriorityChange,
                                                                 saving = false,
                                                                 collapsible = false,
                                                                 expanded = true,
                                                                 onToggleExpanded,
                                                                 extraActions,
                                                                 extensionsCard,
                                                                 onUpdateRecord,
                                                                 onSwitchRoutingMode,
                                                             }) => {
    const smartRouting = record.smartRouting || [];
    const isExpanded = !collapsible || expanded;

    const getApiStyle = (providerUuid: string) => {
        const provider = providers.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    };

    // Same priority-sort the normal RoutingGraph applies, so the visual
    // sequence of default providers matches the runtime failover order.
    const sortedDefaultProviders = React.useMemo(() => {
        const list = record.providers;
        const hasAnyPriority = list.some((p) => (p.priority ?? 0) > 0);
        if (!hasAnyPriority) return list;
        return [...list].sort((a, b) => {
            const ap = a.priority ?? 0;
            const bp = b.priority ?? 0;
            if (ap === 0 && bp !== 0) return 1;
            if (bp === 0 && ap !== 0) return -1;
            return bp - ap;
        });
    }, [record.providers]);

    return (
        <StyledCard active={active}>
            {/* Header Section */}
            <SummarySection
                collapsible={collapsible}
                onClick={collapsible ? onToggleExpanded : undefined}
            >
                {/* Left side */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1, minWidth: 0 }}>
                    <Tooltip title={record.requestModel
                        ? `Use "${record.requestModel}" as model name in your API requests. (click to copy)`
                        : 'No model specified'}>
                        <Typography
                            onClick={(e) => {
                                e.stopPropagation();
                                if (record.requestModel) {
                                    void navigator.clipboard.writeText(record.requestModel);
                                }
                            }}
                            sx={{
                                fontFamily: 'monospace',
                                fontSize: '0.875rem',
                                fontWeight: 600,
                                color: active ? 'text.primary' : 'text.disabled',
                                opacity: active ? 1 : 0.5,
                                cursor: record.requestModel ? 'pointer' : 'default',
                                '&:hover': record.requestModel ? {
                                    color: 'primary.dark',
                                } : {},
                            }}
                        >
                            model = {record.requestModel || 'Unspecified'}
                        </Typography>
                    </Tooltip>
                    {/* Rule Description - moved to top bar with auto-truncate, after Rules chip */}
                    {record.description && (
                        <Tooltip title={record.description} arrow>
                            <Typography
                                variant="caption"
                                sx={{
                                    color: 'text.secondary',
                                    fontSize: '0.75rem',
                                    fontStyle: 'italic',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                    maxWidth: '400px',
                                }}
                            >
                                {record.description}
                            </Typography>
                        </Tooltip>
                    )}
                    {active && record.providers.length === 0 && (
                        <Tooltip title="No default providers - please add default providers to confirm rule works">
                            <WarningIcon
                                sx={{
                                    fontSize: '1.1rem',
                                    color: 'warning.main',
                                    animation: 'pulse 2s ease-in-out infinite',
                                    '@keyframes pulse': {
                                        '0%, 100%': { opacity: 1 },
                                        '50%': { opacity: 0.5 },
                                    },
                                }}
                            />
                        </Tooltip>
                    )}
                </Box>
                {/* Right side */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Box onClick={(e) => e.stopPropagation()}>{extraActions}</Box>
                    {record.responseModel && (
                        <Chip
                            label={`Response as ${record.responseModel}`}
                            size="small"
                            color="info"
                            onClick={(e) => e.stopPropagation()}
                            sx={{
                                opacity: active ? 1 : 0.5,
                                backgroundColor: active ? 'info.main' : 'action.disabled',
                                color: active ? 'info.contrastText' : 'text.disabled'
                            }}
                        />
                    )}
                    {collapsible && onToggleExpanded && (
                        <IconButton
                            size="small"
                            onClick={(e) => {
                                e.stopPropagation();
                                onToggleExpanded();
                            }}
                            sx={{
                                transition: 'transform 0.2s',
                                transform: isExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
                            }}
                        >
                            <ExpandMoreIcon />
                        </IconButton>
                    )}
                </Box>
            </SummarySection>

            {/* Graph Content */}
            <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                <CardContent sx={{ pt: 0, pb: 0.25, '&:last-child': { pb: 0.25 } }}>
                    <Stack spacing={graph.stackSpacing}>
                        {/* Graph row: scrollable graph + pinned extensions card */}
                        <Box sx={{ display: 'flex', alignItems: 'stretch', minWidth: 0 }}>
                            <Box sx={{ flexGrow: 1, minWidth: 0, overflowX: 'auto' }}>
                            <GraphContainer>
                                <GraphRow sx={{ alignItems: 'flex-start' }}>
                                    {/* Request Model section - label + node + arrow as a unit */}
                                    <Box sx={{
                                        display: 'flex',
                                        flexDirection: 'column',
                                        alignItems: 'center',
                                        pr: 1,
                                    }}>
                                        <NodeContainer>
                                            <Tooltip title="The model name that clients use to make requests." placement="top" arrow>
                                                <Box>
                                                    <ModelNode
                                                        active={active}
                                                        label="Unspecified"
                                                        value={record.requestModel}
                                                        editable={active}
                                                        onUpdate={(value) => {
                                                            onUpdateRecord?.('requestModel', value);
                                                        }}
                                                        showSmartSwitch={true}
                                                        smartEnabled={true}
                                                        switchDisabled={saving}
                                                        onSwitch={() => onSwitchRoutingMode?.()}
                                                    />
                                                </Box>
                                            </Tooltip>
                                        </NodeContainer>
                                    </Box>

                                    {/* Arrow to rules section - aligned to center of ModelNode */}
                                    <Box sx={{ flex: 0, display: 'flex', alignItems: 'center', height: MODEL_NODE_STYLES.height }}>
                                        <ArrowNode direction="forward" flowing={false} flowSpeed={1.} />
                                    </Box>

                                    {/* Rules section on the right */}
                                    <Box sx={{
                                        flex: 1,
                                        display: 'flex',
                                        alignItems: "flex-start",
                                        flexDirection: 'column',
                                        gap: 1.5
                                    }}>
                                        {/* Smart Rules */}
                                        {smartRouting.length > 0 ? (
                                            smartRouting.map((rule, index) => (
                                                <React.Fragment key={rule.uuid}>
                                                    <GraphRow>
                                                        {/* Smart Node */}
                                                        <NodeContainer>
                                                            <SmartOpNode
                                                                smartRouting={rule}
                                                                index={index}
                                                                active={active}
                                                                onEdit={() => {
                                                                    onEditSmartRule(rule.uuid);
                                                                }}
                                                                onDelete={() => {
                                                                    onDeleteSmartRule(rule.uuid);
                                                                }}
                                                            />
                                                        </NodeContainer>

                                                        {/* Arrow to providers */}
                                                        <ArrowNode direction="forward" flowing={false} flowSpeed={1.} />

                                                        {/* Providers for this smart rule */}
                                                        {rule.services && rule.services.length > 0 ? (
                                                            <Box sx={{
                                                                display: 'flex',
                                                                gap: 1.5,
                                                                flexWrap: 'nowrap',
                                                                justifyContent: 'flex-start',
                                                                alignItems: 'center'
                                                            }}>
                                                                {rule.services.map((service, serviceIndex) => (
                                                                    <Tooltip
                                                                        key={service.uuid}
                                                                        title={
                                                                            rule.services && rule.services.length >= 2
                                                                                ? `Service ${serviceIndex + 1} of ${rule.services.length} (requests are load balanced)`
                                                                                : 'Service for this smart rule'
                                                                        }
                                                                        placement="top"
                                                                        arrow
                                                                    >
                                                                        <Box>
                                                                            <ProviderNode
                                                                                provider={service}
                                                                                apiStyle={getApiStyle(service.provider)}
                                                                                providersData={providers}
                                                                                active={active && service.active !== false}
                                                                                onDelete={() => {
                                                                                    onDeleteServiceFromSmartRule(rule.uuid, service.uuid);
                                                                                }}
                                                                                onNodeClick={() => onProviderNodeClick?.(service.uuid)}
                                                                            />
                                                                        </Box>
                                                                    </Tooltip>
                                                                ))}
                                                                {/* Add Service Button */}
                                                                <ActionAddNode
                                                                    active={active}
                                                                    onAdd={() => onAddServiceToSmartRule(index)}
                                                                    tooltip="Add service to this smart rule"
                                                                />
                                                            </Box>
                                                        ) : (
                                                            <ActionAddNode
                                                                active={active}
                                                                onAdd={() => onAddServiceToSmartRule(index)}
                                                                tooltip="Add service to this smart rule"
                                                            />
                                                        )}
                                                    </GraphRow>
                                                </React.Fragment>
                                            ))
                                        ) : (
                                            <Box sx={{
                                                display: 'flex',
                                                alignItems: 'flex-start',
                                                justifyContent: 'flex-start',
                                                py: 4
                                            }}>
                                                <Typography variant="body2" color="text.secondary">
                                                    No smart rules configured.
                                                </Typography>
                                            </Box>
                                        )}

                                        {/* Add Smart Rule Button */}
                                        <Box sx={{ display: 'flex', justifyContent: 'flex-start', py: 1 }}>
                                            <Button
                                                variant="outlined"
                                                startIcon={<AddIcon />}
                                                onClick={onAddSmartRule}
                                                disabled={!active}
                                                sx={(theme) => ({
                                                    borderColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.72 : 0.82),
                                                    color: getRouteGraphActiveColor(theme),
                                                    backgroundColor: 'transparent',
                                                    '&:hover': {
                                                        borderColor: getRouteGraphActiveColor(theme),
                                                        backgroundColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.12 : 0.06),
                                                        boxShadow: `0 0 0 3px ${alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.14 : 0.10)}`,
                                                    },
                                                    '&.Mui-disabled': {
                                                        borderColor: theme.palette.divider,
                                                        color: theme.palette.text.disabled,
                                                    },
                                                })}
                                            >
                                                Add Smart Rule
                                            </Button>
                                        </Box>

                                        {/* Default Providers Section - always show even with 0 providers */}
                                        <GraphRow>
                                            {/* Default Node */}
                                            <NodeContainer>
                                                <SmartDefaultNode
                                                    providersCount={record.providers.length}
                                                    active={active}
                                                    onAddProvider={() => onAddDefaultProvider?.()}
                                                />
                                            </NodeContainer>

                                            {/* Arrow to providers */}
                                            <ArrowNode direction="forward" flowing={false} flowSpeed={1.} />

                                            {/* Default Providers */}
                                            <Box sx={{
                                                display: 'flex',
                                                gap: 1.5,
                                                flexWrap: 'nowrap',
                                                justifyContent: 'flex-start',
                                                alignItems: 'center'
                                            }}>
                                                {sortedDefaultProviders.map((provider, providerIndex) => (
                                                    <Tooltip
                                                        key={provider.uuid}
                                                        title={
                                                            (provider.priority ?? 0) > 0
                                                                ? `Priority ${provider.priority} (higher = tried first)`
                                                                : record.providers.length >= 2
                                                                    ? `Default provider ${providerIndex + 1} of ${record.providers.length} (requests are load balanced)`
                                                                    : 'Default provider for request forwarding'
                                                        }
                                                        placement="top"
                                                        arrow
                                                    >
                                                        <Box>
                                                            <ProviderNode
                                                                provider={provider}
                                                                apiStyle={getApiStyle(provider.provider)}
                                                                providersData={providers}
                                                                active={active && provider.active !== false}
                                                                onDelete={() => {
                                                                    onDeleteDefaultProvider?.(provider.uuid);
                                                                }}
                                                                onNodeClick={() => onProviderNodeClick?.(provider.uuid)}
                                                                onPriorityChange={
                                                                    onProviderPriorityChange
                                                                        ? (priority) => onProviderPriorityChange(provider.uuid, priority)
                                                                        : undefined
                                                                }
                                                            />
                                                        </Box>
                                                    </Tooltip>
                                                ))}
                                                {/* Add Provider Button */}
                                                <ActionAddNode
                                                    active={active}
                                                    warning={record.providers.length === 0}
                                                    onAdd={() => onAddDefaultProvider?.()}
                                                    tooltip="Add default provider"
                                                />
                                            </Box>
                                        </GraphRow>
                                    </Box>
                                </GraphRow>
                            </GraphContainer>
                            </Box>
                            {/* Rule Extensions slot - pinned to the right of the rule group, outside horizontal scroll */}
                            {extensionsCard && (
                                <Box
                                    onClick={(e) => e.stopPropagation()}
                                    sx={(theme) => ({
                                        display: 'flex',
                                        alignItems: 'center',
                                        flexShrink: 0,
                                        alignSelf: 'stretch',
                                        ml: 1.5,
                                        pl: 2,
                                        pr: `${graphContainer.marginX}px`,
                                        py: `${graphContainer.marginY}px`,
                                        borderLeft: '1px solid',
                                        borderColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.28 : 0.18),
                                        backgroundImage: `linear-gradient(90deg, ${alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.07 : 0.045)}, transparent 18px)`,
                                    })}
                                >
                                    {extensionsCard}
                                </Box>
                            )}
                        </Box>
                    </Stack>
                </CardContent>
            </Collapse>
        </StyledCard>
    );
};

export default SmartRoutingGraph;
