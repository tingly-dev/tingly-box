import { Box, type SxProps } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { UnifiedRoutingGraph } from '@/components/UnifiedRoutingGraph';
import type { ConfigRecord } from '@/components/RoutingGraphTypes';
import type { Provider } from '@/types/provider';
import { TIER_DIAGRAM_DATA } from './diagrams';

export interface StaticGraphViewerProps {
    scenario: string;
    interactive?: boolean;
    sx?: SxProps;
}

/**
 * StaticGraphViewer - Renders a non-functional UnifiedRoutingGraph for demonstration purposes.
 *
 * This component wraps UnifiedRoutingGraph with demo-mode settings that:
 * - Disable all action callbacks (no actual configuration changes)
 * - Preserve hover states and tooltips for interactivity
 * - Show the visual structure of tier configurations
 */
export const StaticGraphViewer: React.FC<StaticGraphViewerProps> = ({
    scenario,
    interactive = false,
    sx,
    ...props
}) => {
    const { t } = useTranslation();
    const diagramData = TIER_DIAGRAM_DATA[scenario];

    // The demo records carry English descriptions as authoring defaults; look
    // them up per scenario so the diagrams read localized like the rest of
    // the guide (falling back to the authored English text).
    const record = React.useMemo<ConfigRecord | undefined>(() => {
        if (!diagramData) return undefined;
        const base = diagramData.record;
        return {
            ...base,
            description: t(`rule.guideDiagrams.${scenario}.description`, { defaultValue: base.description }),
            smartRouting: base.smartRouting?.map((rule, i) => ({
                ...rule,
                description: t(`rule.guideDiagrams.${scenario}.smart.${i}`, { defaultValue: rule.description }),
            })),
        };
    }, [diagramData, scenario, t]);

    if (!diagramData || !record) {
        return (
            <Box sx={{ textAlign: 'center', color: 'text.secondary', ...sx }} {...props}>
                Diagram not found: {scenario}
            </Box>
        );
    }

    const {
        providers,
        active = true,
    } = diagramData;

    // Create demo callbacks that do nothing
    const handleUpdateRecord = () => {};
    const handleProviderNodeClick = () => {};
    const handleTierChange = () => {};
    const handleDeleteProvider = () => {};
    const handleAddService = () => {};
    const handleAddSmartRule = () => {};
    const handleEditSmartRule = () => {};
    const handleDeleteSmartRule = () => {};
    const handleMoveSmartRule = () => {};
    const handleAddServiceToSmartRule = () => {};
    const handleDeleteServiceFromSmartRule = () => {};
    const handleSwitchRoutingMode = () => {};

    return (
        <Box
            sx={{
                width: '100%',
                maxWidth: '100%',
                minWidth: 300,
                ...sx,
            }}
            {...props}
        >
            <UnifiedRoutingGraph
                mode="auto"
                record={record}
                providers={providers}
                active={active}
                saving={false}
                expanded={true}
                collapsible={false}
                guideMode={true}
                autoScroll={true}
                onUpdateRecord={handleUpdateRecord}
                onProviderNodeClick={handleProviderNodeClick}
                onTierChange={handleTierChange}
                onDeleteProvider={handleDeleteProvider}
                onAddService={handleAddService}
                onAddSmartRule={handleAddSmartRule}
                onEditSmartRule={handleEditSmartRule}
                onDeleteSmartRule={handleDeleteSmartRule}
                onMoveSmartRule={handleMoveSmartRule}
                onAddServiceToSmartRule={handleAddServiceToSmartRule}
                onDeleteServiceFromSmartRule={handleDeleteServiceFromSmartRule}
                onSwitchRoutingMode={handleSwitchRoutingMode}
            />
        </Box>
    );
};

export default StaticGraphViewer;
