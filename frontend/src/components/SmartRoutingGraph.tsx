import {
    Add as AddIcon,
    ArrowDownward as ArrowDownIcon,
    Info as InfoIcon,
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
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React from 'react';
import type { Provider } from '../types/provider';
import { SmartOpNode, ActionAddNode, SmartDefaultNode, ConnectionLine, ModelNode, NodeContainer, ProviderNode } from '@/components/nodes';
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
    providerUuidToName: { [uuid: string]: string };
    active: boolean;
    onAddSmartRule: () => void;
    onEditSmartRule: (ruleUuid: string) => void;
    onDeleteSmartRule: (ruleUuid: string) => void;
    onAddServiceToSmartRule: (ruleUuid: string) => void;
    onAddDefaultProvider?: () => void;
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

const SummarySection = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: `${header.paddingY}px ${header.paddingX}px`,
    borderBottom: '1px solid',
    borderBottomColor: 'divider',
}));

const GraphContainer = styled(Box)(({ theme }) => ({
    padding: `${graphContainer.paddingY}px ${graphContainer.paddingX}px`,
    backgroundColor: 'grey.50',
    borderRadius: theme.shape.borderRadius,
    margin: `${graphContainer.marginY}px ${graphContainer.marginX}px 0`,
}));

const GraphRow = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'stretch',
    justifyContent: 'center',
    gap: graph.rowGap,
    marginBottom: theme.spacing(1),
}));

const SmartRoutingGraph: React.FC<SmartRoutingGraphProps> = ({
    record,
    providers,
    providerUuidToName,
    active,
    onAddSmartRule,
    onEditSmartRule,
    onDeleteSmartRule,
    onAddServiceToSmartRule,
    onAddDefaultProvider,
}) => {
    const smartRouting = record.smartRouting || [];

    const getApiStyle = (providerUuid: string) => {
        const provider = providers.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    };

    return (
        <StyledCard active={active}>
            {/* Header Section */}
            <SummarySection>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1 }}>
                    <Typography variant="h6" sx={{ fontWeight: 600 }}>
                        {record.requestModel || 'Unnamed Model'}
                    </Typography>
                    <Chip
                        label="Smart Routing"
                        size="small"
                        color="primary"
                        variant="outlined"
                        sx={{
                            opacity: active ? 1 : 0.5,
                            borderColor: active ? 'primary.main' : 'text.disabled',
                            minWidth: 90,
                            fontWeight: 600,
                            fontSize: '0.75rem',
                        }}
                    />
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
                    {record.description && (
                        <Typography variant="body2" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                            {record.description}
                        </Typography>
                    )}
                </Box>
            </SummarySection>

            {/* Graph Content */}
            <Box sx={{ overflowX: 'auto' }}>
                <GraphContainer>
                    <Stack spacing={1}>
                        {/* Main layout: Model Node on left, Rules on right */}
                        <Box sx={{ display: 'flex', flexDirection: 'row', alignItems: 'flex-start' }}>
                        {/* Model Node on the left */}
                        <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', pr: 1 }}>
                            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: graph.iconGap, mb: graph.labelMargin }}>
                                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                    Request Model
                                </Typography>
                                <Tooltip title="The model name that clients use to make requests.">
                                    <InfoIcon sx={{ fontSize: '0.9rem', color: 'text.secondary', cursor: 'help' }} />
                                </Tooltip>
                            </Box>
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
                        </Box>

                        {/* Rules section on the right */}
                        <Box sx={{ flex: 1, display: 'flex', alignItems:"flex-start", flexDirection: 'column', gap: 1.5, marginLeft: 3 }}>
                            {/* Smart Rules */}
                            {smartRouting.length > 0 ? (
                                smartRouting.map((rule) => (
                                    <React.Fragment key={rule.uuid}>
                                        <GraphRow>
                                            {/* Smart Node */}
                                            <NodeContainer>
                                                <SmartOpNode
                                                    smartRouting={rule}
                                                    active={active}
                                                    onEdit={() => onEditSmartRule(rule.uuid)}
                                                    onDelete={() => onDeleteSmartRule(rule.uuid)}
                                                    onAddService={() => onAddServiceToSmartRule(rule.uuid)}
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
                                                                console.log('Delete service:', service.uuid);
                                                            }}
                                                            onRefreshModels={(p) => {
                                                                console.log('Refresh models:', p.uuid);
                                                            }}
                                                            providerUuidToName={providerUuidToName}
                                                            onNodeClick={() => {
                                                                console.log('Provider node click:', service.uuid);
                                                            }}
                                                        />
                                                    ))}
                                                    {/* Add Service Button */}
                                                    <ActionAddNode
                                                        active={active}
                                                        onAdd={() => onAddServiceToSmartRule(rule.uuid)}
                                                        tooltip="Add service to this smart rule"
                                                    />
                                                </Box>
                                            ) : (
                                                <ActionAddNode
                                                    active={active}
                                                    onAdd={() => onAddServiceToSmartRule(rule.uuid)}
                                                    tooltip="Add service to this smart rule"
                                                />
                                            )}
                                        </GraphRow>
                                    </React.Fragment>
                                ))
                            ) : (
                                <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'center', py: 4 }}>
                                    <Typography variant="body2" color="text.secondary">
                                        No smart rules configured.
                                    </Typography>
                                </Box>
                            )}

                            {/* Add Smart Rule Button */}
                            <Box sx={{ display: 'flex', justifyContent: 'center', py: 1 }}>
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

                            {/* Default Providers Section */}
                            {record.providers.length > 0 && (
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
                                                    console.log('Delete default provider:', provider.uuid);
                                                }}
                                                onRefreshModels={(p) => {
                                                    console.log('Refresh models:', p.uuid);
                                                }}
                                                providerUuidToName={providerUuidToName}
                                                onNodeClick={() => {
                                                    console.log('Provider node click:', provider.uuid);
                                                }}
                                            />
                                        ))}
                                        {/* Add Provider Button */}
                                        <ActionAddNode
                                            active={active}
                                            warning
                                            onAdd={() => onAddDefaultProvider?.()}
                                            tooltip="Add default provider"
                                        />
                                    </Box>
                                </GraphRow>
                            )}
                        </Box>
                    </Box>
                </Stack>
            </GraphContainer>
            </Box>
        </StyledCard>
    );
};

export default SmartRoutingGraph;
