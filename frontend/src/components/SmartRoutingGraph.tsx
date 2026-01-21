import {
    Add as AddIcon,
    ArrowDownward as ArrowDownIcon,
    Info as InfoIcon,
    SmartDisplay as SmartIcon,
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
import { styled } from '@mui/material/styles';
import React from 'react';
import type { Provider } from '../types/provider';
import { SmartOpNode, ActionAddNode, SmartFallbackNode, ConnectionLine, ModelNode, NodeContainer, ProviderNode } from '@/components/nodes';
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
        paddingY: 8,
    },
    graphContainer: {
        paddingX: 16,
        paddingY: 10,
        marginX: 16,
        marginY: 8,
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
    onToggleSmartEnabled?: (enabled: boolean) => void;
    onProviderNodeClick?: (providerUuid: string) => void;
    // Additional props matching RoutingGraph
    saving?: boolean;
    collapsible?: boolean;
    allowToggleRule?: boolean;
    expanded?: boolean;
    onToggleExpanded?: () => void;
    extraActions?: React.ReactNode;
    onUpdateRecord?: (field: keyof ConfigRecord, value: any) => void;
}

// Styled Card matching RuleGraph style
const StyledCard = styled(Card, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    transition: 'all 0.2s ease-in-out',
    opacity: active ? 1 : 0.6,
    filter: active ? 'none' : 'grayscale(0.3)',
    '&:hover': {
        boxShadow: active ? theme.shadows[4] : theme.shadows[1],
    },
}));

const SummarySection = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'collapsible',
})<{ collapsible?: boolean }>(({ theme, collapsible }) => ({
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
    marginBottom: theme.spacing(1),
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
    onToggleSmartEnabled,
    onProviderNodeClick,
    saving = false,
    collapsible = false,
    allowToggleRule = true,
    expanded = true,
    onToggleExpanded,
    extraActions,
    onUpdateRecord,
}) => {
    const smartRouting = record.smartRouting || [];
    const isExpanded = !collapsible || expanded;

    const getApiStyle = (providerUuid: string) => {
        const provider = providers.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    };

    return (
        <StyledCard active={active}>
            {/* Header Section */}
            <SummarySection
                collapsible={collapsible}
                onClick={collapsible ? onToggleExpanded : undefined}
            >
                {/* Left side */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1 }}>
                    <Tooltip title={record.requestModel
                        ? `Use "${record.requestModel}" as model name in your API requests. (click to copy)`
                        : 'No model specified'}>
                        <Chip
                            label={`model = ${record.requestModel || 'Unspecified'}`}
                            size="small"
                            variant="outlined"
                            onClick={(e) => {
                                e.stopPropagation();
                                if (record.requestModel) {
                                    void navigator.clipboard.writeText(record.requestModel);
                                }
                            }}
                            sx={{
                                opacity: active ? 1 : 0.5,
                                borderColor: active ? 'primary.main' : 'text.disabled',
                                color: active ? 'primary.main' : 'text.disabled',
                                minWidth: 150,
                                fontWeight: 600,
                                cursor: record.requestModel ? 'pointer' : 'default',
                                '& .MuiChip-label': {
                                    fontWeight: 600,
                                },
                            }}
                        />
                    </Tooltip>
                    {/* Active/Inactive Toggle */}
                    <Tooltip title={
                        saving || !allowToggleRule
                            ? 'Cannot toggle status while saving'
                            : active
                                ? 'Click to deactivate'
                                : 'Click to activate'
                    }>
                        <Chip
                            label={active ? "Active" : "Inactive"}
                            size="small"
                            color={active ? "success" : "default"}
                            variant={active ? "filled" : "outlined"}
                            onClick={(e) => {
                                e.stopPropagation();
                                if (!saving && allowToggleRule && onUpdateRecord) {
                                    onUpdateRecord('active', !active);
                                }
                            }}
                            sx={{
                                opacity: active ? 1 : 0.7,
                                minWidth: 75,
                                cursor: (saving || !allowToggleRule) ? 'default' : 'pointer',
                                '&:hover': (saving || !allowToggleRule) ? {} : {
                                    opacity: 0.8,
                                },
                            }}
                        />
                    </Tooltip>
                    <Chip
                        label={`${smartRouting.length} ${smartRouting.length === 1 ? 'Rule' : 'Rules'}`}
                        size="small"
                        variant="outlined"
                        sx={{
                            opacity: active ? 1 : 0.5,
                            borderColor: active ? 'inherit' : 'text.disabled',
                            color: active ? 'inherit' : 'text.disabled',
                        }}
                    />
                    {active && record.providers.length === 0 && (
                        <Tooltip title="No fallback providers - please add fallback providers to confirm rule works">
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
                    {/* Smart Toggle Button */}
                    <Tooltip title="Switch to normal routing mode">
                        <Chip
                            icon={<SmartIcon fontSize="small" />}
                            label="Smart"
                            size="small"
                            color="primary"
                            variant="filled"
                            onClick={(e) => {
                                e.stopPropagation();
                                onToggleSmartEnabled?.(false);
                            }}
                            sx={{
                                opacity: active ? 1 : 0.5,
                                minWidth: 75,
                                cursor: 'pointer',
                                '&:hover': {
                                    opacity: 0.8,
                                },
                            }}
                        />
                    </Tooltip>
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
                {/* Description - Full width below */}
                <Box
                    onClick={(e) => e.stopPropagation()}
                    sx={{
                        width: '100%',
                        flexBasis: '100%',
                        mt: 0.5,
                        minHeight: '18px',
                    }}
                >
                    {record.description && (
                        <Typography
                            variant="body2"
                            sx={{
                                color: 'text.secondary',
                                fontSize: '0.8rem',
                                fontStyle: 'italic',
                            }}
                        >
                            {record.description}
                        </Typography>
                    )}
                </Box>
            </SummarySection>

            {/* Graph Content */}
            <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                <CardContent sx={{ pt: 0, pb: 1 }}>
                    <Stack spacing={graph.stackSpacing}>
                        {/* Graph Visualization */}
                        <Box sx={{ overflowX: 'auto' }}>
                            <GraphContainer>
                                <GraphRow>
                                    {/* Request Model section - label + node + arrow as a unit */}
                                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', pr: 1, mt: -1 }}>
                                        {/* Request Model Label */}
                                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: graph.iconGap, mb: graph.labelMargin }}>
                                            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                                Request Model
                                            </Typography>
                                            <Tooltip title="The model name that clients use to make requests.">
                                                <InfoIcon sx={{ fontSize: '0.9rem', color: 'text.secondary', cursor: 'help' }} />
                                            </Tooltip>
                                        </Box>
                                        {/* Node + Arrow as a row */}
                                        <Box sx={{ display: 'flex', flexDirection: 'row', alignItems: 'center' }}>
                                            <NodeContainer>
                                                <ModelNode
                                                    active={active}
                                                    label="Unspecified"
                                                    value={record.requestModel}
                                                    editable={active}
                                                    onUpdate={(value) => {
                                                        console.log('Update request model:', value);
                                                    }}
                                                />
                                            </NodeContainer>
                                            {/* Arrow to rules section */}
                                            <ConnectionLine>
                                                <ArrowDownIcon sx={{ transform: 'rotate(270deg)' }} />
                                            </ConnectionLine>
                                        </Box>
                                    </Box>

                        {/* Rules section on the right */}
                        <Box sx={{ flex: 1, display: 'flex', alignItems:"flex-start", flexDirection: 'column', gap: 1.5 }}>
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
                                                        console.log('SmartRoutingGraph: onEdit called for rule:', rule.uuid, rule.description);
                                                        onEditSmartRule(rule.uuid);
                                                    }}
                                                    onDelete={() => {
                                                        console.log('SmartRoutingGraph: onDelete called for rule:', rule.uuid);
                                                        onDeleteSmartRule(rule.uuid);
                                                    }}
                                                />
                                            </NodeContainer>

                                            {/* Arrow to providers */}
                                            <ConnectionLine>
                                                <ArrowDownIcon sx={{ transform: 'rotate(270deg)' }} />
                                            </ConnectionLine>

                                            {/* Providers for this smart rule */}
                                            {rule.services && rule.services.length > 0 ? (
                                                <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'nowrap', justifyContent: 'flex-start', alignItems: 'center' }}>
                                                    {rule.services.map((service) => (
                                                        <ProviderNode
                                                            key={service.uuid}
                                                            provider={service}
                                                            apiStyle={getApiStyle(service.provider)}
                                                            providersData={providers}
                                                            active={active && service.active !== false}
                                                            onDelete={() => {
                                                                console.log('SmartRoutingGraph: onDelete clicked for service:', service.uuid, 'rule:', rule.uuid, 'active:', active, 'service.active:', service.active);
                                                                onDeleteServiceFromSmartRule(rule.uuid, service.uuid);
                                                            }}
                                                            onNodeClick={() => onProviderNodeClick?.(service.uuid)}
                                                        />
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
                                <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'flex-start', py: 4 }}>
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
                                    sx={{
                                        borderColor: 'primary.main',
                                        color: 'primary.main',
                                        '&:hover': {
                                            borderColor: 'primary.dark',
                                            backgroundColor: 'primary.50',
                                        },
                                    }}
                                >
                                    Add Smart Rule
                                </Button>
                            </Box>

                            {/* Default Providers Section - always show even with 0 providers */}
                            <GraphRow>
                                {/* Default Node */}
                                <NodeContainer>
                                    <SmartFallbackNode
                                        providersCount={record.providers.length}
                                        active={active}
                                        onAddProvider={() => onAddDefaultProvider?.()}
                                    />
                                </NodeContainer>

                                {/* Arrow to providers */}
                                <ConnectionLine>
                                    <ArrowDownIcon sx={{ transform: 'rotate(270deg)' }} />
                                </ConnectionLine>

                                {/* Default Providers */}
                                <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'nowrap', justifyContent: 'flex-start', alignItems: 'center' }}>
                                    {record.providers.map((provider) => (
                                        <ProviderNode
                                            key={provider.uuid}
                                            provider={provider}
                                            apiStyle={getApiStyle(provider.provider)}
                                            providersData={providers}
                                            active={active && provider.active !== false}
                                            onDelete={() => {
                                                onDeleteDefaultProvider?.(provider.uuid);
                                            }}
                                            onNodeClick={() => onProviderNodeClick?.(provider.uuid)}
                                        />
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
        </Stack>
                    </CardContent>
                </Collapse>
            </StyledCard>
        );
    };

export default SmartRoutingGraph;
