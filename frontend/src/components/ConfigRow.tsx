import {Box, Stack, Typography} from '@mui/material';
import React, {type ReactNode} from 'react';

// ============================================================================
// Types
// ============================================================================

export type TabKey = string;

export interface ConfigTab {
    key: TabKey;
    label: string;
    content: ReactNode;
    actions?: ReactNode;
}

interface ConfigRowProps {
    /** Tabs configuration (single tab = single row, multiple tabs = | separated) */
    tabs: ConfigTab[];
    /** Currently active tab key */
    activeTab: TabKey;
    /** Callback when tab changes */
    onTabChange: (tabKey: TabKey) => void;
    /** Maximum width of the row (default: 700) */
    maxWidth?: number;
}

// ============================================================================
// Tab Button Component
// ============================================================================

const TabButton: React.FC<{
    label: string;
    isActive: boolean;
    onClick: () => void;
}> = ({label, isActive, onClick}) => (
    <Box
        onClick={onClick}
        sx={{
            px: 1.5,
            py: 0.5,
            fontSize: '0.8125rem',
            fontWeight: isActive ? 700 : 500,
            cursor: 'pointer',
            userSelect: 'none',
            transition: 'all 0.15s ease-in-out',
            '&:hover': {
                backgroundColor: 'action.hover',
                borderRadius: 0.5,
            },
        }}
    >
        {label}
    </Box>
);

// ============================================================================
// Main Component
// ============================================================================

/**
 * Unified configuration row component.
 * All rows use tab mode - single tab shows just content, multiple tabs show | separator.
 *
 * Single tab example:
 * ┌──────────────────────────────────────────────────────┐
 * │ Label    Content (flex: 1)           Actions         │
 * └──────────────────────────────────────────────────────┘
 *
 * Multiple tabs example:
 * ┌─────────────────────────────────────────────────────────┐
 * │ Tab A | Tab B    Content (flex: 1)           Actions     │
 * └─────────────────────────────────────────────────────────┘
 */
export const ConfigRow: React.FC<ConfigRowProps> = ({
                                                        tabs,
                                                        activeTab,
                                                        onTabChange,
                                                        maxWidth = 700,
                                                    }) => {
    // Get current tab data
    const currentTab = tabs.find(t => t.key === activeTab) || tabs[0];

    // Build left side: tabs with | separator
    const leftContent = (
        <Box sx={{display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0}}>
            {tabs.map((tab, index) => (
                <React.Fragment key={tab.key}>
                    <TabButton
                        label={tab.label}
                        isActive={activeTab === tab.key}
                        onClick={() => onTabChange(tab.key)}
                    />
                    {index < tabs.length - 1 && (
                        <Typography sx={{fontSize: '0.8125'}}>|</Typography>
                    )}
                </React.Fragment>
            ))}
        </Box>
    );

    return (
        <Box sx={{display: 'flex', alignItems: 'center', gap: 3, maxWidth}}>
            {/* Left: Tabs (with | separator if multiple) */}
            <Box sx={{minWidth: "200px"}}>
                {leftContent}
            </Box>

            {/* Center: Content */}
            <Box sx={{flex: 1, minWidth: 0}}>
                {currentTab?.content}
            </Box>

            {/* Right: Actions */}
            {currentTab?.actions && (
                <Stack direction="row" spacing={0.5} sx={{flexShrink: 0, ml: 'auto'}}>
                    {currentTab.actions}
                </Stack>
            )}
        </Box>
    );
};

export default ConfigRow;
