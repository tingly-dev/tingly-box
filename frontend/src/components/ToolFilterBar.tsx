/**
 * ToolFilterBar
 *
 * A segmented "All / Active / Off" filter pill group with an optional
 * "Expand all / Collapse all" text toggle on the right side.
 *
 * Usage:
 *   const [filter, setFilter] = useState<ToolFilter>('all');
 *   const [allExpanded, setAllExpanded] = useState(true);
 *
 *   <ToolFilterBar filter={filter} onFilterChange={setFilter}
 *                  allExpanded={allExpanded} onToggleExpand={setAllExpanded} />
 *
 *   // Apply filter to your card list:
 *   const visible = cards.filter(c =>
 *     filter === 'all' || (filter === 'active' ? c.enabled : !c.enabled)
 *   );
 */

import React from 'react';
import { Box } from '@mui/material';

export type ToolFilter = 'all' | 'active' | 'off';

interface ToolFilterBarProps {
    filter: ToolFilter;
    onFilterChange: (f: ToolFilter) => void;
    allExpanded: boolean;
    onToggleExpand: (v: boolean) => void;
}

const PILLS: { value: ToolFilter; label: string }[] = [
    { value: 'all', label: 'All' },
    { value: 'active', label: 'Active' },
    { value: 'off', label: 'Off' },
];

export const ToolFilterBar: React.FC<ToolFilterBarProps> = ({
    filter,
    onFilterChange,
    allExpanded,
    onToggleExpand,
}) => (
    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 1, mb: 1.5 }}>
        {/* Expand / Collapse all — same pill-track style */}
        <Box
            sx={{
                display: 'flex',
                bgcolor: 'rgb(238, 233, 224)',
                borderRadius: '8px',
                p: '3px',
                alignItems: 'center',
            }}
        >
            <Box
                component="button"
                onClick={() => onToggleExpand(!allExpanded)}
                sx={{
                    height: '25.5px',
                    px: '10px',
                    py: '5px',
                    borderRadius: '5px',
                    border: 'none',
                    cursor: 'pointer',
                    bgcolor: 'transparent',
                    color: 'rgb(107, 114, 128)',
                    fontSize: '0.8125rem',
                    fontWeight: 600,
                    lineHeight: 1,
                    transition: 'color 0.15s',
                    '&:hover': { color: 'rgb(13, 17, 23)' },
                    '&:focus-visible': { outline: '2px solid rgb(10, 124, 90)', outlineOffset: '1px' },
                }}
            >
                {allExpanded ? 'Collapse all' : 'Expand all'}
            </Box>
        </Box>

        {/* Segmented pill filter */}
        <Box
            sx={{
                display: 'flex',
                flexDirection: 'row',
                gap: '6px',
                bgcolor: 'rgb(238, 233, 224)',
                borderRadius: '8px',
                p: '3px',
                alignItems: 'center',
            }}
        >
            {PILLS.map(({ value, label }) => {
                const selected = filter === value;
                return (
                    <Box
                        key={value}
                        component="button"
                        onClick={() => onFilterChange(value)}
                        sx={{
                            height: '25.5px',
                            px: '10px',
                            py: '5px',
                            borderRadius: '5px',
                            border: 'none',
                            cursor: 'pointer',
                            bgcolor: selected ? 'rgb(255, 255, 255)' : 'transparent',
                            color: selected ? 'rgb(13, 17, 23)' : 'rgb(107, 114, 128)',
                            fontSize: '0.8125rem',
                            fontWeight: 600,
                            lineHeight: 1,
                            boxShadow: selected ? 'rgba(0, 0, 0, 0.06) 0px 1px 2px 0px' : 'none',
                            transition: 'background-color 0.15s, color 0.15s, box-shadow 0.15s',
                            '&:focus-visible': { outline: '2px solid rgb(10, 124, 90)', outlineOffset: '1px' },
                        }}
                    >
                        {label}
                    </Box>
                );
            })}
        </Box>
    </Box>
);

export default ToolFilterBar;
