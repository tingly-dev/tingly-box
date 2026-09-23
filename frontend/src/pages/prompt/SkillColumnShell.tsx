import { Paper } from '@mui/material';
import type { ReactNode } from 'react';

interface SkillColumnShellProps {
    width?: number;
    flex?: number;
    header: ReactNode;
    children: ReactNode;
}

/**
 * Shared Paper shell for the three-column Skill page layout:
 * a bordered column with a fixed header strip and a scrollable body.
 */
const SkillColumnShell = ({ width, flex, header, children }: SkillColumnShellProps) => (
    <Paper
        sx={{
            width,
            flex,
            display: 'flex',
            flexDirection: 'column',
            border: 1,
            borderColor: 'divider',
            borderRadius: 2,
            overflow: 'hidden',
        }}
    >
        {header}
        {children}
    </Paper>
);

export default SkillColumnShell;
