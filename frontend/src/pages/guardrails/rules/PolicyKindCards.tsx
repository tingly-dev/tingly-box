import { Box, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { ArticleOutlined, CheckCircleRounded, HelpOutline, Terminal } from '@/components/icons';
import type { EditorState } from './types';

const kindCards: Array<{
    value: 'resource_access' | 'command_execution' | 'content';
    label: string;
    icon: typeof Terminal;
    tooltip: string;
    // The Command Execution label is long enough that it must not wrap.
    labelNoWrap?: boolean;
}> = [
    {
        value: 'resource_access',
        label: 'Resource Access',
        icon: Terminal,
        tooltip: 'Inspect access to files, directories, and other resources. Best for reads, writes, deletes, and protected paths like ~/.ssh or .env.',
    },
    {
        value: 'command_execution',
        label: 'Command Execution',
        icon: Terminal,
        tooltip: 'Inspect commands the model wants to run. Best for dangerous shell commands, execution patterns, and risky programs like rm -rf or curl | sh.',
        labelNoWrap: true,
    },
    {
        value: 'content',
        label: 'Privacy Policy',
        icon: ArticleOutlined,
        tooltip: 'Inspect returned text from the model or tools. Use privacy patterns to review or block sensitive content.',
    },
];

type PolicyKindCardsProps = {
    kind: EditorState['kind'];
    onSelect: (kind: 'resource_access' | 'command_execution' | 'content') => void;
};

const PolicyKindCards = ({ kind, onSelect }: PolicyKindCardsProps) => (
    <Box
        sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', md: '1fr', lg: '1fr 1fr 1fr' },
            gap: 2,
        }}
    >
        {kindCards.map((card) => {
            const Icon = card.icon;
            const selected = kind === card.value;
            return (
                <Box
                    key={card.value}
                    onClick={() => onSelect(card.value)}
                    sx={{
                        border: '1px solid',
                        borderColor: selected ? 'primary.main' : 'divider',
                        bgcolor: selected ? 'action.selected' : 'background.paper',
                        borderRadius: 2,
                        p: 2,
                        cursor: 'pointer',
                        transition: 'all 0.15s ease',
                        '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                    }}
                >
                    <Stack spacing={1}>
                        <Stack
                            direction="row"
                            spacing={1}
                            useFlexGap
                            sx={{
                                alignItems: "center",
                                flexWrap: "wrap"
                            }}>
                            <Icon fontSize="small" color={selected ? 'primary' : 'action'} />
                            <Typography
                                variant="subtitle2"
                                sx={card.labelNoWrap ? { whiteSpace: 'nowrap' } : undefined}
                            >
                                {card.label}
                            </Typography>
                            <Tooltip title={card.tooltip}>
                                <IconButton size="small" sx={{ p: 0.25 }}>
                                    <HelpOutline fontSize="inherit" />
                                </IconButton>
                            </Tooltip>
                            {selected && (
                                <Tooltip title="Selected">
                                    <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                </Tooltip>
                            )}
                        </Stack>
                    </Stack>
                </Box>
            );
        })}
    </Box>
);

export default PolicyKindCards;
