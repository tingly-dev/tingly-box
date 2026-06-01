import {
    Add as AddIcon,
    ExpandMore as ExpandMoreIcon,
} from '@/components/icons';
import {
    Box,
    Button,
    Card,
    CardContent,
    Collapse,
    IconButton,
    Stack,
} from '@mui/material';
import { alpha, styled } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { getRouteGraphActiveColor, SMART_NODE_STYLES } from '@/components/nodes/styles';
import {
    ActionAddNode,
    ArrowNode,
    DividerNode,
    NodeContainer,
    TierNode,
    TIER_NODE_WIDTH,
    ServiceNode,
    SmartOpNode,
    ServiceEntryNode,
} from '@/components/nodes';
import { EntryNode } from '@/components/nodes';
import ModelRequestHeader from '@/components/ModelRequestHeader';
import type { Provider } from '../types/provider';
import type { ConfigRecord } from './RoutingGraphTypes';

// Routing mode controls display behavior
export type RoutingMode = 'smart' | 'direct' | 'auto';

// Unified style configuration
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
        paddingY: 6,
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

export interface UnifiedRoutingGraphProps {
    // Mode control
    mode?: RoutingMode;

    // Data
    record: ConfigRecord;
    providers: Provider[];

    // State
    active?: boolean;
    saving?: boolean;
    expanded?: boolean;
    collapsible?: boolean;

    // Callbacks
    onUpdateRecord?: (field: keyof ConfigRecord, value: any) => void;
    onProviderNodeClick?: (providerUuid: string) => void;
    onProviderPriorityChange?: (providerUuid: string, priority: number) => void;
    onDeleteProvider?: (providerUuid: string) => void;
    onAddProvider?: (priority?: number) => void;
    onMoveTier?: (tierPriority: number, direction: 'up' | 'down') => void;
    onToggleExpanded?: () => void;

    // Smart routing callbacks
    onAddSmartRule?: () => void;
    onEditSmartRule?: (ruleUuid: string) => void;
    onDeleteSmartRule?: (ruleUuid: string) => void;
    onMoveSmartRule?: (ruleUuid: string, direction: 'up' | 'down') => void;
    onAddServiceToSmartRule?: (ruleUuid: string) => void;
    onDeleteServiceFromSmartRule?: (ruleUuid: string, serviceUuid: string) => void;

    // Routing mode switch
    onSwitchRoutingMode?: () => void;

    // Slots
    extraActions?: React.ReactNode;
    extensionsCard?: React.ReactNode;
}

// Styled components
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

const GraphContainer = styled(Box)(({ theme }) => ({
    padding: `${graphContainer.paddingY}px ${graphContainer.paddingX}px`,
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

/**
 * UnifiedRoutingGraph - A single component that handles both smart and direct routing modes.
 *
 * Replaces SmartRoutingGraph and RoutingGraph with mode-based display control.
 */
export const UnifiedRoutingGraph: React.FC<UnifiedRoutingGraphProps> = ({
    mode = 'auto',
    record,
    providers,
    active = true,
    saving = false,
    expanded = true,
    collapsible = false,
    onUpdateRecord,
    onProviderNodeClick,
    onProviderPriorityChange,
    onDeleteProvider,
    onAddProvider,
    onMoveTier,
    onToggleExpanded,
    onAddSmartRule,
    onEditSmartRule,
    onDeleteSmartRule,
    onMoveSmartRule,
    onAddServiceToSmartRule,
    onDeleteServiceFromSmartRule,
    onSwitchRoutingMode,
    extraActions,
    extensionsCard,
}) => {
    const { t } = useTranslation();
    const isExpanded = !collapsible || expanded;

    // Determine effective mode
    const smartEnabled = record.smartEnabled || false;
    const effectiveMode: 'smart' | 'direct' = mode === 'auto'
        ? (smartEnabled ? 'smart' : 'direct')
        : mode;

    const smartRouting = record.smartRouting || [];
    const hasSmartRules = smartRouting.length > 0;
    const showSmartRouting = effectiveMode === 'smart' || (mode === 'auto' && smartEnabled && hasSmartRules);

    const getApiStyle = React.useCallback((providerUuid: string) => {
        const provider = providers.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    }, [providers]);

    // Priority-sorted default providers
    const sortedDefaultProviders = React.useMemo(() => {
        const list = record.providers;
        const hasAnyPriority = list.some((p) => (p.priority ?? 0) > 0);
        if (!hasAnyPriority) return list;
        return [...list].sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0));
    }, [record.providers]);

    // Group already-sorted providers into priority tiers (single pass — order preserved from sortedDefaultProviders)
    const priorityGroups = React.useMemo(() => {
        const groups = new Map<number, typeof sortedDefaultProviders>();
        for (const p of sortedDefaultProviders) {
            const tier = p.priority ?? 0;
            if (!groups.has(tier)) groups.set(tier, []);
            groups.get(tier)!.push(p);
        }
        return [...groups.entries()].map(([priority, providers]) => ({ priority, providers }));
    }, [sortedDefaultProviders]);

    // Handle add service to smart rule
    const handleAddServiceToSmartRule = React.useCallback((ruleIndex: number) => {
        if (onAddServiceToSmartRule) {
            const rule = smartRouting[ruleIndex];
            if (rule) {
                onAddServiceToSmartRule(rule.uuid);
            }
        }
    }, [smartRouting, onAddServiceToSmartRule]);

    // Non-zero priority groups (sorted ascending by priority value = P1, P2, P3...)
    const nonZeroPriorityGroups = React.useMemo(
        () => priorityGroups.filter((g) => g.priority > 0),
        [priorityGroups],
    );

    // Unset-priority services (priority === 0)
    const zeroGroup = React.useMemo(
        () => priorityGroups.find((g) => g.priority === 0) ?? null,
        [priorityGroups],
    );

    const isPriorityMode = nonZeroPriorityGroups.length > 0;

    // Render a single ServiceNode (shared between both layouts)
    const renderServiceNode = React.useCallback(
        (provider: typeof sortedDefaultProviders[0], hidePriorityBadge = false) => (
            <ServiceNode
                key={provider.uuid}
                provider={provider}
                apiStyle={getApiStyle(provider.provider)}
                providersData={providers}
                active={active && provider.active !== false}
                onDelete={() => onDeleteProvider?.(provider.uuid)}
                onNodeClick={() => onProviderNodeClick?.(provider.uuid)}
                onPriorityChange={
                    onProviderPriorityChange
                        ? (priority) => onProviderPriorityChange(provider.uuid, priority)
                        : undefined
                }
                showPriority={!hidePriorityBadge}
            />
        ),
        [getApiStyle, providers, active, onDeleteProvider, onProviderNodeClick, onProviderPriorityChange],
    );

    // Priority tier layout: stacked rows, one per tier, with TierNode on the left
    const renderPriorityTierLayout = React.useCallback(() => {
        const maxPriority = nonZeroPriorityGroups.length > 0
            ? nonZeroPriorityGroups[nonZeroPriorityGroups.length - 1].priority
            : 0;

        return (
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                {nonZeroPriorityGroups.map((group, idx) => (
                    <Box
                        key={group.priority}
                        sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flexWrap: 'nowrap' }}
                    >
                        <TierNode
                            tierIndex={idx}
                            priority={group.priority}
                            active={active}
                            canMoveUp={idx > 0}
                            canMoveDown={idx < nonZeroPriorityGroups.length - 1}
                            onMoveUp={() => onMoveTier?.(group.priority, 'up')}
                            onMoveDown={() => onMoveTier?.(group.priority, 'down')}
                        />
                        <ArrowNode direction="forward" />
                        {group.providers.map((p) => renderServiceNode(p, true))}
                        <ActionAddNode
                            active={active && !saving}
                            onAdd={() => onAddProvider?.(group.priority)}
                            tooltip={t('rule.tooltips.addServiceSecond')}
                        />
                    </Box>
                ))}

                {/* Unset-tier row (priority=0 services) — no tier node */}
                {zeroGroup && (
                    <Box
                        key={0}
                        sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flexWrap: 'nowrap' }}
                    >
                        <Box sx={{ width: TIER_NODE_WIDTH, flexShrink: 0 }} />
                        <ArrowNode direction="forward" />
                        {zeroGroup.providers.map((p) => renderServiceNode(p, true))}
                    </Box>
                )}

                {/* Add new tier button */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    <Box sx={{ width: TIER_NODE_WIDTH, flexShrink: 0 }} />
                    <ActionAddNode
                        active={active && !saving}
                        onAdd={() => onAddProvider?.(maxPriority + 1)}
                        tooltip={t('rule.tier.addTierTooltip')}
                    />
                </Box>
            </Box>
        );
    }, [t, nonZeroPriorityGroups, zeroGroup, active, saving, renderServiceNode, onMoveTier, onAddProvider]);

    // Flat service list (no tiers): horizontal inline layout with dividers between groups
    const renderProviderList = React.useCallback(() => {
        const hasMultipleTiers = priorityGroups.length > 1;

        return (
            <>
                {hasMultipleTiers ? (
                    priorityGroups.map((group, groupIndex) => (
                        <React.Fragment key={group.priority}>
                            {groupIndex > 0 && <DividerNode active={active} />}
                            {group.providers.map((p) => renderServiceNode(p))}
                        </React.Fragment>
                    ))
                ) : (
                    sortedDefaultProviders.map((p) => renderServiceNode(p))
                )}
                <ActionAddNode
                    active={active && !saving}
                    warning={record.providers.length === 0}
                    onAdd={() => onAddProvider?.()}
                    tooltip={
                        record.providers.length === 0
                            ? t('rule.tooltips.addServiceFirst')
                            : t('rule.tooltips.addServiceSecond')
                    }
                />
            </>
        );
    }, [t, priorityGroups, sortedDefaultProviders, active, saving, record.providers.length, renderServiceNode, onAddProvider]);

    // Render smart rules section
    const renderSmartRules = () => {
        if (!showSmartRouting) return null;

        return (
            <>
                {/* Smart Rules List */}
                {smartRouting.length > 0 ? (
                    smartRouting.map((rule, index) => (
                        <React.Fragment key={rule.uuid}>
                            <GraphRow>
                                <NodeContainer>
                                    <SmartOpNode
                                        smartRouting={rule}
                                        index={index}
                                        active={active}
                                        onEdit={() => onEditSmartRule?.(rule.uuid)}
                                        onDelete={() => onDeleteSmartRule?.(rule.uuid)}
                                        onMoveUp={index > 0 ? () => onMoveSmartRule?.(rule.uuid, 'up') : undefined}
                                        onMoveDown={index < smartRouting.length - 1 ? () => onMoveSmartRule?.(rule.uuid, 'down') : undefined}
                                    />
                                </NodeContainer>

                                <ArrowNode direction="forward" />

                                {rule.services && rule.services.length > 0 ? (
                                    <Box sx={{
                                        display: 'flex',
                                        gap: 1.5,
                                        flexWrap: 'nowrap',
                                        justifyContent: 'flex-start',
                                        alignItems: 'center'
                                    }}>
                                        {rule.services.map((service) => (
                                            <ServiceNode
                                                key={service.uuid}
                                                provider={service}
                                                apiStyle={getApiStyle(service.provider)}
                                                providersData={providers}
                                                active={active && service.active !== false}
                                                onDelete={() => onDeleteServiceFromSmartRule?.(rule.uuid, service.uuid)}
                                                onNodeClick={() => onProviderNodeClick?.(service.provider)}
                                            />
                                        ))}
                                        <ActionAddNode
                                            active={active}
                                            onAdd={() => handleAddServiceToSmartRule(index)}
                                            tooltip="Add service to this smart rule"
                                        />
                                    </Box>
                                ) : (
                                    <ActionAddNode
                                        active={active}
                                        onAdd={() => handleAddServiceToSmartRule(index)}
                                        tooltip="Add service to this smart rule"
                                    />
                                )}
                            </GraphRow>
                        </React.Fragment>
                    ))
                ) : null}

                {/* Add Smart Rule Button */}
                <Box sx={{ display: 'flex', justifyContent: 'flex-start', py: 2 }}>
                    <Button
                        variant="outlined"
                        startIcon={<AddIcon />}
                        onClick={onAddSmartRule}
                        disabled={!active || saving}
                        sx={(theme) => ({
                            width: SMART_NODE_STYLES.width,
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
            </>
        );
    };

    // Render default providers section
    const renderDefaultProviders = () => {
        return (
            <GraphRow sx={{ alignItems: 'flex-start' }}>
                <NodeContainer>
                    <ServiceEntryNode
                        providersCount={record.providers.length}
                        active={active}
                    />
                </NodeContainer>

                <Box sx={{ flex: 0, display: 'flex', alignItems: 'center', height: 72 }}>
                    <ArrowNode direction="forward" />
                </Box>

                {isPriorityMode ? (
                    renderPriorityTierLayout()
                ) : (
                    <Box sx={{
                        display: 'flex',
                        gap: 1.5,
                        flexWrap: 'nowrap',
                        justifyContent: 'flex-start',
                        alignItems: 'center',
                    }}>
                        {renderProviderList()}
                    </Box>
                )}
            </GraphRow>
        );
    };

    return (
        <StyledCard active={active}>
            {/* Header Section - Using ModelRequestHeader with all elements integrated */}
            <ModelRequestHeader
                modelName={record.requestModel || 'Unspecified'}
                onModelChange={(value) => onUpdateRecord?.('requestModel', value)}
                editable={active}
                active={active}
                subtitle={record.description}
                collapsible={collapsible}
                onClick={collapsible ? onToggleExpanded : undefined}
                extraActions={extraActions}
                isExpanded={isExpanded}
                onToggleExpanded={onToggleExpanded}
            />

            {/* Graph Content */}
            <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                <CardContent sx={{ pt: 0, pb: 0.25, '&:last-child': { pb: 0.25 } }}>
                    <Stack spacing={graph.stackSpacing}>
                        {/* Graph row: scrollable graph + pinned extensions card */}
                        <Box sx={{ display: 'flex', alignItems: 'stretch', minWidth: 0 }}>
                            <Box sx={{ flexGrow: 1, minWidth: 0, overflowX: 'auto' }}>
                                <GraphContainer>
                                    <GraphRow sx={{ alignItems: 'flex-start' }}>
                                        {/* EntryNode - Direct/Smart mode selector */}
                                        <Box sx={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            pr: 1,
                                        }}>
                                            <EntryNode
                                                active={active}
                                                smartEnabled={smartEnabled}
                                                onSwitch={onSwitchRoutingMode}
                                                switchDisabled={saving}
                                            />
                                        </Box>

                                        {/* Arrow - height matches EntryNode so it stays centered under flex-start */}
                                        <Box sx={{ flex: 0, display: 'flex', alignItems: 'center', height: 72 }}>
                                            <ArrowNode direction="forward" />
                                        </Box>

                                        {/* Smart Routing Section (conditional) */}
                                        {showSmartRouting ? (
                                            <Box sx={{
                                                flex: 1,
                                                display: 'flex',
                                                alignItems: "flex-start",
                                                flexDirection: 'column',
                                                gap: 1.5
                                            }}>
                                                {renderSmartRules()}
                                                {renderDefaultProviders()}
                                            </Box>
                                        ) : isPriorityMode ? (
                                            /* Direct + Priority Mode: stacked tier rows */
                                            renderPriorityTierLayout()
                                        ) : (
                                            /* Direct Mode: providers inline */
                                            <Box sx={{ display: 'flex', flexWrap: 'nowrap', gap: 1.5, alignItems: 'center' }}>
                                                {renderProviderList()}
                                            </Box>
                                        )}
                                    </GraphRow>
                                </GraphContainer>
                            </Box>

                            {/* Extensions Card Slot */}
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

export default UnifiedRoutingGraph;
