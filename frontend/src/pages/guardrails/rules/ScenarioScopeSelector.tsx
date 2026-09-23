import { Box, FormHelperText, Stack, Tooltip, Typography } from '@mui/material';
import {
    AutoAwesome,
    CheckCircleRounded,
    Code as CodeIcon,
    LaptopMac,
    Rule,
} from '@/components/icons';
import { Anthropic, Claude, OpenAI } from '@/components/BrandIcons';
import { toggleValue } from './listField';

const getScenarioPresentation = (scenario: string) => {
    switch (scenario) {
        case 'anthropic':
            return {
                label: 'Anthropic',
                description: 'Anthropic-compatible requests and responses.',
                icon: <Anthropic size={18} />,
            };
        case 'claude_code':
            return {
                label: 'Claude Code',
                description: 'Tool-enabled Claude Code sessions and command workflows.',
                icon: <Claude size={18} />,
            };
        case 'openai':
            return {
                label: 'OpenAI',
                description: 'OpenAI-compatible requests and responses.',
                icon: <OpenAI size={18} />,
            };
        case 'opencode':
            return {
                label: 'OpenCode',
                description: 'OpenCode scenario traffic and agent flows.',
                icon: <CodeIcon sx={{ fontSize: 18 }} />,
            };
        case 'xcode':
            return {
                label: 'Xcode',
                description: 'Xcode-integrated coding workflows.',
                icon: <LaptopMac sx={{ fontSize: 18 }} />,
            };
        case 'custom':
            return {
                label: 'Custom',
                description: 'Bring-your-own-request-model custom scenario traffic.',
                icon: <AutoAwesome sx={{ fontSize: 18 }} />,
            };
        default: {
            const label = scenario
                .split('_')
                .filter(Boolean)
                .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
                .join(' ');
            return {
                label,
                description: `${label} scenario traffic.`,
                icon: <Rule sx={{ fontSize: 18 }} color="action" />,
            };
        }
    }
};

type ScenarioScopeSelectorProps = {
    title: string;
    description: string;
    scenarioOptions: string[];
    value: string[];
    onChange: (value: string[]) => void;
    helperText: string;
};

const ScenarioScopeSelector = ({
    title,
    description,
    scenarioOptions,
    value,
    onChange,
    helperText,
}: ScenarioScopeSelectorProps) => (
    <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
        <Stack spacing={1.5}>
            <Box>
                <Typography variant="subtitle2">{title}</Typography>
                <Typography
                    variant="caption"
                    sx={{
                        color: "text.secondary",
                        display: 'block',
                        mt: 0.5
                    }}>
                    {description}
                </Typography>
            </Box>
            <Box
                sx={{
                    display: 'grid',
                    gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                    gap: 1.5,
                }}
            >
                {scenarioOptions.map((option) => {
                    const selected = value.includes(option);
                    const presentation = getScenarioPresentation(option);
                    return (
                        <Box
                            key={option}
                            onClick={() => onChange(toggleValue(value, option))}
                            sx={{
                                border: '1px solid',
                                borderColor: selected ? 'primary.main' : 'divider',
                                bgcolor: selected ? 'action.selected' : 'background.paper',
                                borderRadius: 2,
                                p: 1.5,
                                cursor: 'pointer',
                                transition: 'all 0.15s ease',
                                '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                            }}
                        >
                            <Stack spacing={0.75}>
                                <Stack
                                    direction="row"
                                    spacing={1}
                                    useFlexGap
                                    sx={{
                                        alignItems: "center",
                                        flexWrap: "wrap"
                                    }}>
                                    {presentation.icon}
                                    <Typography variant="body2" sx={{
                                        fontWeight: 600
                                    }}>
                                        {presentation.label}
                                    </Typography>
                                    {selected && (
                                        <Tooltip title="Selected">
                                            <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                        </Tooltip>
                                    )}
                                </Stack>
                                <Typography variant="caption" sx={{
                                    color: "text.secondary"
                                }}>
                                    {presentation.description}
                                </Typography>
                            </Stack>
                        </Box>
                    );
                })}
            </Box>
            <FormHelperText>{helperText}</FormHelperText>
        </Stack>
    </Box>
);

export default ScenarioScopeSelector;
