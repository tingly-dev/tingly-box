import { Stack, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';

interface SectionHeaderProps {
    title: string;
    subtitle: string;
    component: 'h1' | 'h2';
    sx?: SxProps<Theme>;
}

// One-line header shared by the agent overview's sections (Agents,
// Power-ups): title and hint share a baseline so each section costs a single
// row above its cards. No icon — the rail already carries it.
const SectionHeader: React.FC<SectionHeaderProps> = ({ title, subtitle, component, sx }) => (
    <Stack
        direction="row"
        sx={{ alignItems: 'baseline', columnGap: 1.5, rowGap: 0.25, flexWrap: 'wrap', mb: 1.5, ...sx }}
    >
        <Typography component={component} variant="h6" sx={{ fontWeight: 600 }}>
            {title}
        </Typography>
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {subtitle}
        </Typography>
    </Stack>
);

export default SectionHeader;
