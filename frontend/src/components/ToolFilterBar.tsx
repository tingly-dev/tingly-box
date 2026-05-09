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
import { Box, Typography } from '@mui/material';

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
    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1.5 }}>
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

        {/* Expand / Collapse all */}
        <Typography
            component="button"
            onClick={() => onToggleExpand(!allExpanded)}
            sx={{
                border: 'none',
                bgcolor: 'transparent',
                cursor: 'pointer',
                fontSize: '0.75rem',
                fontWeight: 500,
                color: 'text.disabled',
                p: 0,
                '&:hover': { color: 'text.secondary' },
                transition: 'color 0.15s',
            }}
        >
            {allExpanded ? 'Collapse all' : 'Expand all'}
        </Typography>
    </Box>
);

export default ToolFilterBar;
