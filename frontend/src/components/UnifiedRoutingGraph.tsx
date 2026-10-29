import {Box, Button, Card, CardContent, Collapse, Stack,} from '@mui/material';
import {alpha, styled} from '@mui/material/styles';
import React, {useEffect, useRef} from 'react';
import {useTranslation} from 'react-i18next';
import {EntryGuideDialog} from '@/components/tier/EntryGuideDialog';
import {
    getRouteGraphActiveColor,
    PROVIDER_NODE_STYLES,
    SMALL_NODE_STYLES,
} from '@/components/nodes/styles';
import {
    ActionAddNode,
    ArrowNode,
    NodeContainer,
    ServiceEntryNode,
    ServiceNode,
    SmartOpNode,
    TierNode,
} from '@/components/nodes';
import NodeTooltip from '@/components/nodes/NodeTooltip';
import ModelRequestHeader from '@/components/ModelRequestHeader';
import {TierGuideDialog} from '@/components/tier/TierGuideDialog';
import type {Provider} from '../types/provider';
import type {ConfigRecord} from './RoutingGraphTypes';
import {useResponsesToggle} from '@/hooks/useResponsesToggle';

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

const {graphContainer, graph} = RULE_GRAPH_STYLES;

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

    // Guide mode - for demo/tier guide display
    guideMode?: boolean;

    // Callbacks
    onUpdateRecord?: (field: keyof ConfigRecord, value: any) => void;
    onServiceNodeClick?: (serviceUuid: string) => void;
    onEditProvider?: (providerUuid: string) => void;
    onTierChange?: (serviceUuid: string, tier: number) => void;
    onDeleteService?: (serviceUuid: string) => void;
    onAddService?: (tier?: number) => void;
    onToggleExpanded?: () => void;

    // Smart routing callbacks
    onAddSmartRule?: () => void;
    onEditSmartRule?: (ruleUuid: string, opUuid?: string) => void;
    onDeleteSmartRule?: (ruleUuid: string) => void;
    onMoveSmartRule?: (ruleUuid: string, direction: 'up' | 'down') => void;
    onAddServiceToSmartRule?: (ruleUuid: string) => void;
    onDeleteServiceFromSmartRule?: (ruleUuid: string, serviceUuid: string) => void;

    // Routing mode switch
    onSwitchRoutingMode?: () => void;

    // 1M context window toggle - receives the new state for scenario-specific handling
    onContext1MToggle?: (newState: boolean, ruleUuid?: string) => void;

    // Slots
    extraActions?: React.ReactNode;
    extensionsCard?: React.ReactNode;
    /** Plugins tag in the rule header (count + click), see ModelRequestHeader. */
    pluginsCount?: number;
    onPluginsClick?: (anchor: HTMLElement) => void;

    // Scroll control: when true, auto-scroll to 80% position (for guide mode)
    autoScroll?: boolean;
}

// Styled components
const StyledCard = styled(Card, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({active, theme}) => ({
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

const GraphContainer = styled(Box)(({theme}) => ({
    padding: `${graphContainer.paddingY}px ${graphContainer.paddingX}px`,
    borderRadius: theme.shape.borderRadius,
    margin: `${graphContainer.marginY}px ${graphContainer.marginX}px 0`,
}));

const GraphRow = styled(Box)(({theme}) => ({
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
                                                                            guideMode = false,
                                                                            onUpdateRecord,
                                                                            onServiceNodeClick,
                                                                            onEditProvider,
                                                                            onTierChange,
                                                                            onDeleteService,
                                                                            onAddService,
                                                                            onToggleExpanded,
                                                                            onAddSmartRule,
                                                                            onEditSmartRule,
                                                                            onDeleteSmartRule,
                                                                            onMoveSmartRule,
                                                                            onAddServiceToSmartRule,
                                                                            onDeleteServiceFromSmartRule,
                                                                            onSwitchRoutingMode,
                                                                            onContext1MToggle,
                                                                            extraActions,
                                                                            extensionsCard,
                                                                            pluginsCount,
                                                                            onPluginsClick,
                                                                            autoScroll,
                                                                        }) => {
    const {t} = useTranslation();
    const isExpanded = !collapsible || expanded;

    // Ref for the scrollable container
    const scrollContainerRef = useRef<HTMLDivElement>(null);

    // Scroll to show the right portion of the graph when scrollAt is true
    useEffect(() => {
        if (isExpanded && autoScroll && scrollContainerRef.current) {
            const container = scrollContainerRef.current;
            // Scroll to show 80% of the content (right portion but not extreme edge)
            const scrollTarget = SMALL_NODE_STYLES.width;
            container.scrollTo({
                left: Math.max(0, scrollTarget),
                behavior: 'smooth',
            });
        }
    }, [isExpanded, autoScroll]);

    // Track which tier is being hovered
    const [hoveredTier, setHoveredTier] = React.useState<number | null>(null);
    const [showTierGuide, setShowTierGuide] = React.useState(false);

    const [entryGuideMode, setEntryGuideMode] = React.useState<'direct' | 'smart' | null>(null);

    // Hover teaching for the Default and Add condition nodes (the old
    // Direct/Smart entry node's tooltip, split per concept), each with a link
    // to the matching guide.
    const guideTooltip = (title: string, body: string, linkLabel: string, mode: 'direct' | 'smart') => (
        <Box sx={{whiteSpace: 'pre-line', maxWidth: 280}}>
            <strong>{title}</strong>
            {`\n${body}`}
            <Box
                component="span"
                onClick={(e) => { e.stopPropagation(); setEntryGuideMode(mode); }}
                sx={{display: 'block', mt: 1, color: 'primary.main', cursor: 'pointer', fontWeight: 500, '&:hover': {textDecoration: 'underline'}}}
            >
                {linkLabel}
            </Box>
        </Box>
    );
    const defaultTooltip = guideTooltip(
        t('rule.routing.directTooltipTitle'), t('rule.routing.directTooltipBody'),
        t('rule.routing.viewDirectGuide'), 'direct');
    const conditionTooltip = guideTooltip(
        t('rule.routing.smartTooltipTitle'), t('rule.routing.smartTooltipBody'),
        t('rule.routing.viewSmartGuide'), 'smart');

    // A new, still empty tier the user just asked for. It lives here, not in
    // the record: tiers are compacted to be contiguous on every save, which
    // would drop an empty one before a model lands in it. Cleared whenever the
    // service list changes (the model was added, or something else happened).
    const [pendingTier, setPendingTier] = React.useState(false);
    useEffect(() => { setPendingTier(false); }, [record.providers.length]);

    const handleShowGuide = () => {
        setShowTierGuide(true);
    };

    const handleGuideClose = () => {
        setShowTierGuide(false);
    };

    // There is no Direct/Smart switch: a rule is its Default row, plus
    // conditions above it when it has any. smartEnabled stays on the record
    // (the backend only evaluates conditions while it is on) and follows the
    // conditions; a rule saved with conditions but smartEnabled off (the old
    // "Direct" toggle) shows them paused with a Resume action.
    const smartEnabled = record.smartEnabled || false;
    const smartRouting = record.smartRouting || [];
    const hasSmartRules = smartRouting.length > 0;
    const showSmartRouting = mode !== 'direct' && hasSmartRules;
    const conditionsPaused = showSmartRouting && !smartEnabled;

    const getApiStyle = React.useCallback((providerUuid: string) => {
        const provider = providers.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    }, [providers]);

    // Tier-sorted default providers
    const sortedDefaultProviders = React.useMemo(() => {
        const list = record.providers;
        const hasTiers = list.some((p) => (p.tier ?? 0) > 0);
        if (!hasTiers) return list;
        return [...list].sort((a, b) => (a.tier ?? 0) - (b.tier ?? 0));
    }, [record.providers]);

    // The rule override is separate from catalog and provider defaults. Auto
    // lets the resolver consult those declarations; forcing Responses is
    // checked against the current upstream before saving.
    const primaryService = sortedDefaultProviders.find((p) => p.active !== false) || sortedDefaultProviders[0];
    // The Endpoint chip is always shown; it only has an effect when some service
    // of the rule (default tiers or a smart rule's) is an OpenAI-compatible upstream.
    const endpointApplicable = [...record.providers, ...(record.smartRouting || []).flatMap((r) => r.services || [])]
        .some((p) => getApiStyle(p.provider) === 'openai');
    const {
        selection: endpointSelection,
        probing: responsesProbing,
        onSelect: handleEndpointSelect,
    } = useResponsesToggle({record, primaryService, onUpdateRecord});

    // Group already-sorted providers into tiers (single pass — order preserved from sortedDefaultProviders)
    const tierGroups = React.useMemo(() => {
        const groups = new Map<number, typeof sortedDefaultProviders>();
        for (const p of sortedDefaultProviders) {
            const tier = p.tier ?? 0;
            if (!groups.has(tier)) groups.set(tier, []);
            groups.get(tier)!.push(p);
        }
        return [...groups.entries()].map(([tier, providers]) => ({tier, providers}));
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

    // Tier layout: all tier groups rendered with TierNode label, always shown.
    // T0 row is always present (even with no providers) to guide users.
    // Each service has up/down arrows; up from T0 is hidden (already highest).
    // Tiers are kept contiguous from 0 (normalizeProviderTiers on every
    // mutation + backend save), so down is also hidden for the sole service
    // of the bottom tier — moving it further would just be renumbered back.
    const renderTierLayout = React.useCallback(() => {
        // Always show at least T0, even when no providers exist
        const baseGroups = tierGroups.length > 0 ? tierGroups : [{tier: 0, providers: [] as typeof sortedDefaultProviders}];
        const lastGroup = baseGroups[baseGroups.length - 1];
        // Only a filled bottom tier can have a new one below it.
        const canAddTier = !guideMode && lastGroup.providers.length > 0;
        const groups = pendingTier && canAddTier
            ? [...baseGroups, {tier: lastGroup.tier + 1, providers: [] as typeof sortedDefaultProviders}]
            : baseGroups;

        // In guide mode, always show action buttons on all services
        const shouldShowActions = guideMode ? true : undefined;

        return (
            <Box sx={{display: 'flex', flexDirection: 'column', gap: 1.5}}>
                {groups.map((group, idx) => {
                    // Under contiguous-tier normalization, moving the sole
                    // service of the bottom tier further down is a no-op
                    // (it would just be renumbered back) — hide its ↓ arrow,
                    // symmetric with ↑ being hidden on T0.
                    const isLoneBottomTier = idx === groups.length - 1 && group.providers.length === 1;
                    return (
                    <Box
                        key={group.tier}
                        sx={{display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'nowrap'}}
                    >
                        <TierNode
                            priority={group.tier}
                            active={active}
                            onHover={guideMode ? undefined : (hovering) => setHoveredTier(hovering ? group.tier : null)}
                            onShowGuide={handleShowGuide}
                            onAddTier={canAddTier && !pendingTier && active && !saving && idx === groups.length - 1
                                ? () => setPendingTier(true) : undefined}
                            addTierLabel={t('rule.tier.addTierTooltip')}
                        />
                        {group.providers.map((p) => (
                            <ServiceNode
                                key={p.uuid}
                                provider={p}
                                apiStyle={getApiStyle(p.provider)}
                                scenario={record.scenario}
                                providersData={providers}
                                active={active && p.active !== false}
                                onDelete={() => onDeleteService?.(p.uuid)}
                                onNodeClick={() => onServiceNodeClick?.(p.uuid)}
                                onEditProvider={onEditProvider}
                                showTier={false}
                                forceShowActions={shouldShowActions ?? (hoveredTier === group.tier)}
                                onMoveTierUp={group.tier > 0 && onTierChange ? () => onTierChange(p.uuid, group.tier - 1) : undefined}
                                onMoveTierDown={onTierChange && !isLoneBottomTier ? () => onTierChange(p.uuid, group.tier + 1) : undefined}
                            />
                        ))}
                        <ActionAddNode
                            active={active && !saving}
                            warning={record.providers.length === 0 && idx === 0}
                            onAdd={() => onAddService?.(group.tier)}
                            tooltip={
                                record.providers.length === 0 && idx === 0
                                    ? t('rule.tooltips.addServiceFirst')
                                    : t('rule.tooltips.addServiceSecond')
                            }
                        />
                    </Box>
                    );
                })}
            </Box>
        );
    }, [pendingTier, t, tierGroups, active, saving, record.providers.length, getApiStyle, providers, onDeleteService, onServiceNodeClick, onEditProvider, onTierChange, onAddService, hoveredTier, guideMode, handleShowGuide]);

    // Render smart rules section
    const renderSmartRules = () => {
        if (!showSmartRouting) return null;

        return (
            <>
                {conditionsPaused && (
                    <Box sx={{display: 'flex', alignItems: 'center', gap: 1, fontSize: 12.5, color: 'text.secondary'}}>
                        {t('rule.ruleConditions.conditionsPaused')}
                        <Button size="small" onClick={onSwitchRoutingMode} disabled={!active || saving}>
                            {t('rule.ruleConditions.conditionsResume')}
                        </Button>
                    </Box>
                )}
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
                                        onEdit={(opUuid) => onEditSmartRule?.(rule.uuid, opUuid)}
                                        onDelete={() => onDeleteSmartRule?.(rule.uuid)}
                                        onMoveUp={index > 0 ? () => onMoveSmartRule?.(rule.uuid, 'up') : undefined}
                                        onMoveDown={index < smartRouting.length - 1 ? () => onMoveSmartRule?.(rule.uuid, 'down') : undefined}
                                    />
                                </NodeContainer>

                                <ArrowNode direction="forward"/>

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
                                                scenario={record.scenario}
                                                providersData={providers}
                                                active={active && service.active !== false}
                                                onDelete={() => onDeleteServiceFromSmartRule?.(rule.uuid, service.uuid)}
                                                onNodeClick={() => onServiceNodeClick?.(service.uuid)}
                                                onEditProvider={onEditProvider}
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

                {/* Add condition (with conditions; without them it sits under Default) */}
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5, py: 1}}>
                    <ActionAddNode
                        label={t('rule.ruleConditions.addCondition')}
                        active={active && !saving}
                        onAdd={() => onAddSmartRule?.()}
                        tooltip={conditionTooltip}
                    />
                    {smartRouting.length > 1 && (
                        <Box sx={{fontSize: 12, color: 'text.secondary'}}>
                            {t('rule.ruleConditions.conditionsHint')}
                        </Box>
                    )}
                </Box>
            </>
        );
    };

    // Render default providers section
    const renderDefaultProviders = () => {
        return (
            <GraphRow sx={{alignItems: 'flex-start'}}>
                {/* Fixed height matches the tier row height so ServiceEntryNode centres vertically */}
                <NodeContainer sx={{height: PROVIDER_NODE_STYLES.height, justifyContent: 'center'}}>
                    {/* No conditions: Add condition sits beside Default (on its right), both a service node tall */}
                    <Box sx={{display: 'flex', gap: 1}}>
                        <ServiceEntryNode
                            providersCount={record.providers.length}
                            active={active}
                            compact={!showSmartRouting}
                            tooltip={defaultTooltip}
                        />
                        {!showSmartRouting && (
                            <ActionAddNode
                                label={t('rule.ruleConditions.addCondition')}
                                square
                                active={active && !saving}
                                onAdd={() => onAddSmartRule?.()}
                                tooltip={conditionTooltip}
                            />
                        )}
                    </Box>
                </NodeContainer>

                <Box sx={{flex: 0, display: 'flex', alignItems: 'center', height: PROVIDER_NODE_STYLES.height}}>
                    <ArrowNode direction="forward"/>
                </Box>

                {renderTierLayout()}
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
                context1M={record.flags?.context1m || false}
                onContext1MToggle={() => {
                    const newState = !(record.flags?.context1m || false);
                    // Update the flags
                    const updatedFlags = { ...record.flags, context1m: newState };
                    onUpdateRecord?.('flags', updatedFlags);
                    // Notify parent for scenario-specific handling
                    onContext1MToggle?.(newState, record.uuid);
                }}
                endpointSelection={endpointSelection}
                responsesProbing={responsesProbing}
                onEndpointSelect={handleEndpointSelect}
                onCompactChange={record.scenario === 'codex' ? (percent) => {
                    onUpdateRecord?.('flags', { ...record.flags, codexCompactPercent: percent || undefined });
                } : undefined}
                compactPercent={record.flags?.codexCompactPercent || 0}
                endpointApplicable={endpointApplicable}
                pluginsCount={pluginsCount}
                onPluginsClick={onPluginsClick}
            />

            {/* Tier Guide Dialog */}
            <TierGuideDialog
                open={showTierGuide}
                onClose={handleGuideClose}
            />

            <EntryGuideDialog
                open={entryGuideMode !== null}
                onClose={() => setEntryGuideMode(null)}
                mode={entryGuideMode ?? 'direct'}
            />

            {/* Graph Content */}
            <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                <CardContent sx={{pt: 0, pb: 0.25, '&:last-child': {pb: 0.25}}}>
                    <Stack spacing={graph.stackSpacing}>
                        {/* Graph row: scrollable graph + pinned extensions card */}
                        <Box sx={{display: 'flex', alignItems: 'stretch', minWidth: 0}}>
                            <Box ref={scrollContainerRef} sx={{flexGrow: 1, minWidth: 0, overflowX: 'auto'}}>
                                <GraphContainer>
                                    <Box sx={{display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 1.5}}>
                                        {renderSmartRules()}
                                        {renderDefaultProviders()}
                                    </Box>
                                </GraphContainer>
                            </Box>

                            {/* Extensions Card Slot */}
                            {extensionsCard && (
                                <Box
                                    onClick={(e) => e.stopPropagation()}
                                    sx={(theme) => ({
                                        display: 'flex',
                                        alignItems: 'flex-start',
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
