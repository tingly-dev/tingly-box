import {
    Add as AddIcon,
    ArrowDownward as ArrowDownIcon,
    Info as InfoIcon,
} from '@mui/icons-material';
import {
    Box,
    Button,
    Chip,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import type { Provider } from '../types/provider';
import { ConnectionLine, ModelNode, NodeContainer, ProviderNodeComponent } from './RuleNode';
import { SmartNode } from './SmartNode';
import type { ConfigRecord, SmartRouting } from './RuleGraphTypes';

// Styles matching RuleGraph for consistency
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
    modelNode: {
        padding: 10,
    },
    providerNode: {
        badgeHeight: 5,
        fieldHeight: 5,
        fieldPadding: 2,
        elementMargin: 1,
    },
} as const;

const { node, spacing, header, graphContainer, graph, modelNode, providerNode } = RULE_GRAPH_STYLES;

interface SmartRoutingGraphProps {
    record: ConfigRecord;
    providers: Provider[];
    providerUuidToName: { [uuid: string]: string };
    active: boolean;
    onToggleSmartEnabled: (enabled: boolean) => void;
    onAddSmartRule: () => void;
    onEditSmartRule: (ruleUuid: string) => void;
    onDeleteSmartRule: (ruleUuid: string) => void;
    onAddServiceToSmartRule: (ruleUuid: string) => void;
}

// Graph Container
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

const StyledCard = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    backgroundColor: 'background.paper',
    borderRadius: theme.shape.borderRadius,
    boxShadow: theme.shadows[1],
    border: '1px solid',
    borderColor: 'divider',
    transition: 'all 0.2s ease-in-out',
    opacity: active ? 1 : 0.8,
}));

const HeaderSection = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: `${header.paddingY}px ${header.paddingX}px`,
    borderBottom: '1px solid',
    borderBottomColor: 'divider',
}));

const SmartRoutingGraph: React.FC<SmartRoutingGraphProps> = ({
    record,
    providers,
    providerUuidToName,
    active,
    onToggleSmartEnabled,
    onAddSmartRule,
    onEditSmartRule,
    onDeleteSmartRule,
    onAddServiceToSmartRule,
}) => {
    const { t } = useTranslation();
    const smartRouting = record.smartRouting || [];
    const hasSmartRules = smartRouting.length > 0;

    const getApiStyle = (providerUuid: string) => {
        const provider = providers.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    };

    return (
        <StyledCard active={active}>
            {/* Header Section */}
            <HeaderSection>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1 }}>
                    <Typography variant="h6" sx={{ fontWeight: 600 }}>
                        {record.requestModel || 'Unnamed Model'}
                    </Typography>
                    <Chip
                        label="Smart Routing"
                        size="small"
                        color="primary"
                        variant="filled"
                        sx={{ minWidth: 90, fontWeight: 600, fontSize: '0.75rem' }}
                    />
                    <Chip
                        label={`${smartRouting.length} ${smartRouting.length === 1 ? 'Rule' : 'Rules'}`}
                        size="small"
                        variant="outlined"
                    />
                    <Chip
                        label={`${record.providers.length} ${record.providers.length === 1 ? 'Provider' : 'Providers'}`}
                        size="small"
                        variant="outlined"
                    />
                    {record.description && (
                        <Typography variant="body2" sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                            {record.description}
                        </Typography>
                    )}
                </Box>
            </HeaderSection>

            {/* Graph Content */}
            <GraphContainer>
                <Stack spacing={1}>
                    {/* Main layout: Model Node on left, Smart Rules on right */}
                    {hasSmartRules ? (
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

                            {/* Smart Rules on the right */}
                            <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
                                {smartRouting.map((rule, index) => (
                                    <React.Fragment key={rule.uuid}>
                                        <GraphRow>
                                            {/* Smart Node */}
                                            <NodeContainer>
                                                <SmartNode
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
                                                <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap', justifyContent: 'center', alignItems: 'center' }}>
                                                    {rule.services.map((service) => (
                                                        <ProviderNodeComponent
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
                                                    <Tooltip title="Add service to this smart rule">
                                                        <IconButton
                                                            onClick={() => onAddServiceToSmartRule(rule.uuid)}
                                                            disabled={!active}
                                                            sx={{
                                                                width: node.width,
                                                                height: node.height,
                                                                border: '2px dashed',
                                                                borderColor: 'divider',
                                                                borderRadius: 2,
                                                                backgroundColor: 'background.paper',
                                                                boxShadow: theme => theme.shadows[2],
                                                                transition: 'all 0.2s ease-in-out',
                                                                display: 'flex',
                                                                flexDirection: 'column',
                                                                justifyContent: 'center',
                                                                alignItems: 'center',
                                                                gap: 1,
                                                                '&:hover': {
                                                                    borderColor: 'primary.main',
                                                                    backgroundColor: 'action.hover',
                                                                    borderStyle: 'solid',
                                                                    boxShadow: theme => theme.shadows[4],
                                                                    transform: 'translateY(-2px)',
                                                                },
                                                            }}
                                                        >
                                                            <AddIcon sx={{ fontSize: 40, color: 'text.secondary' }} />
                                                            <Typography variant="body2" color="text.secondary" textAlign="center">
                                                                Add Service
                                                            </Typography>
                                                        </IconButton>
                                                    </Tooltip>
                                                </Box>
                                            ) : (
                                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                                    <Typography variant="body2" color="text.secondary">
                                                        No services
                                                    </Typography>
                                                    <Tooltip title="Add first service">
                                                        <IconButton
                                                            onClick={() => onAddServiceToSmartRule(rule.uuid)}
                                                            disabled={!active}
                                                            size="small"
                                                        >
                                                            <AddIcon fontSize="small" />
                                                        </IconButton>
                                                    </Tooltip>
                                                </Box>
                                            )}
                                        </GraphRow>
                                    </React.Fragment>
                                ))}
                            </Box>
                        </Box>
                    ) : (
                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', py: 4 }}>
                            <Typography variant="body2" color="text.secondary">
                                No smart rules configured. Add your first rule below.
                            </Typography>
                        </Box>
                    )}

                    {/* Add Smart Rule Button */}
                    <Box sx={{ display: 'flex', justifyContent: 'center', pt: 2 }}>
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

                    {/* Default Providers Section (Fallback) */}
                    {record.providers.length > 0 && (
                        <>
                            <Box sx={{ display: 'flex', justifyContent: 'center', pt: 1 }}>
                                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                    ─── Default Providers (Fallback) ───
                                </Typography>
                            </Box>
                            <GraphRow>
                                <Box sx={{ flex: 1, display: 'flex', gap: 1.5, flexWrap: 'wrap', justifyContent: 'center', alignItems: 'center' }}>
                                    {record.providers.map((provider) => (
                                        <ProviderNodeComponent
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
                                </Box>
                            </GraphRow>
                        </>
                    )}
                </Stack>
            </GraphContainer>
        </StyledCard>
    );
};

export default SmartRoutingGraph;
