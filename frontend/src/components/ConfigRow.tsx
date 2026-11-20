import {Box, Stack, Typography} from '@mui/material';
import React, {type ReactNode} from 'react';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { usePhoneShell } from '@tingly/ui/hooks/usePhoneShell';

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
    /** Maximum width of the row (default: responsive - no fixed limit) */
    maxWidth?: number | 'responsive';
}

// ============================================================================
// Tab Button Component
// ============================================================================

const TabButton: React.FC<{
    label: string;
    isActive: boolean;
    onClick: () => void;
}> = ({label, isActive, onClick}) => {
    const phone = usePhoneShell();
    return (
        <Box
            onClick={onClick}
            sx={{
                px: 1.5,
                py: 0.5,
                fontSize: fontSizes.md,
                // Keep fontWeight fixed so the label's rendered width never
                // changes on selection — toggling font-weight here shifts every
                // tab/separator to its right. Fake the bold via text-shadow
                // instead, which affects stroke, not glyph metrics.
                fontWeight: 500,
                ...(phone && {whiteSpace: 'nowrap'}),
                color: isActive ? 'text.primary' : 'text.secondary',
                textShadow: isActive ? '0 0 0.4px currentColor, 0 0 0.4px currentColor' : 'none',
                cursor: 'pointer',
                userSelect: 'none',
                transition: 'all 0.15s ease-in-out',
                '&:hover': {
                    backgroundColor: 'action.hover',
                    borderRadius: radius.chip,
                },
            }}
        >
            {label}
        </Box>
    );
};

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
                                                        maxWidth = 'responsive',
                                                    }) => {
    // Get current tab data
    const currentTab = tabs.find(t => t.key === activeTab) || tabs[0];
    const phone = usePhoneShell();

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

    // Responsive width: use maxWidth if provided as number, otherwise use 100% with breakpoints
    const widthStyle = maxWidth === 'responsive'
        ? {
            width: '100%',
            maxWidth: { xs: '100%', sm: '100%', md: '100%', lg: '100%', xl: 1200 }
          }
        : { maxWidth };

    // Phone shell: the label, copy and switches share the first line; the
    // value gets the full width below, wrapped instead of cut to "http://l…".
    if (phone) {
        return (
            <Box sx={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) auto', alignItems: 'center', columnGap: 1, width: '100%'}}>
                <Box sx={{minWidth: 0}}>{leftContent}</Box>
                {currentTab?.actions ? (
                    <Stack direction="row" spacing={0.5} sx={{justifySelf: 'end', alignItems: 'center'}}>
                        {currentTab.actions}
                    </Stack>
                ) : <span />}
                <Box
                    sx={{
                        gridColumn: '1 / -1',
                        minWidth: 0,
                        '& .MuiTypography-root': {whiteSpace: 'normal', overflowWrap: 'anywhere', overflow: 'visible', textOverflow: 'clip'},
                    }}
                >
                    {currentTab?.content}
                </Box>
            </Box>
        );
    }

    return (
        <Box
            sx={{
                display: 'grid',
                alignItems: 'center',
                columnGap: {xs: 1, sm: 3},
                rowGap: {xs: 0.5, sm: 0},
                gridTemplateColumns: {
                    xs: 'minmax(0, 1fr) auto',
                    sm: '168px minmax(0, 1fr) auto',
                },
                ...widthStyle,
            }}
        >
            {/* Left: Tabs (with | separator if multiple) */}
            <Box sx={{gridColumn: {xs: '1 / -1', sm: '1'}, minWidth: 0}}>
                {leftContent}
            </Box>

            {/* Center: Content */}
            <Box sx={{gridColumn: {xs: '1', sm: '2'}, minWidth: 0, overflow: 'hidden'}}>
                {currentTab?.content}
            </Box>

            {/* Right: Actions */}
            {currentTab?.actions && (
                <Stack
                    direction="row"
                    spacing={0.5}
                    sx={{
                        gridColumn: {xs: '2', sm: '3'},
                        justifySelf: 'end',
                        alignItems: 'center',
                    }}
                >
                    {currentTab.actions}
                </Stack>
            )}
        </Box>
    );
};

export default ConfigRow;
