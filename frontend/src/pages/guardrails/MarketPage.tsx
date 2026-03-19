import { useEffect, useMemo, useState, type ReactNode } from 'react';
import {
    Alert,
    Box,
    Button,
    Chip,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import {
    AddShoppingCart,
    Rule,
    Folder,
    Terminal,
    Shield,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

type RuleTemplate = {
    key: string;
    icon: ReactNode;
    title: string;
    description: string;
    category: string;
    payload: any;
};

const ruleTemplates: RuleTemplate[] = [
    {
        key: 'block-ssh-read',
        icon: <Folder sx={{ fontSize: 28, color: 'primary.main' }} />,
        title: 'Block SSH directory reads',
        description: 'Prevents shell commands from reading `~/.ssh` and `/etc/ssh` paths before local execution.',
        category: 'Sensitive Paths',
        payload: {
            id: 'block-ssh-read',
            name: 'Block SSH directory reads',
            enabled: true,
            kind: 'resource_access',
            group: '',
            scope: {
                scenarios: ['claude_code'],
            },
            verdict: 'block',
            reason: 'This policy blocks attempts to read SSH configuration and key directories.',
            match: {
                tool_names: ['bash'],
                actions: { include: ['read'] },
                resources: {
                    type: 'path',
                    mode: 'prefix',
                    values: ['~/.ssh', '/etc/ssh'],
                },
            },
        },
    },
    {
        key: 'block-env-read',
        icon: <Shield sx={{ fontSize: 28, color: 'primary.main' }} />,
        title: 'Block .env file reads',
        description: 'Blocks common attempts to inspect `.env` files and other environment secret files through shell tools.',
        category: 'Secrets',
        payload: {
            id: 'block-env-read',
            name: 'Block .env file reads',
            enabled: true,
            kind: 'resource_access',
            group: '',
            scope: {
                scenarios: ['claude_code'],
            },
            verdict: 'block',
            reason: 'This policy blocks attempts to read environment variable files that may contain secrets.',
            match: {
                tool_names: ['bash'],
                actions: { include: ['read'] },
                resources: {
                    type: 'path',
                    mode: 'contains',
                    values: ['.env', '.env.local', '.env.production'],
                },
            },
        },
    },
    {
        key: 'block-shell-history-read',
        icon: <Terminal sx={{ fontSize: 28, color: 'primary.main' }} />,
        title: 'Block shell history reads',
        description: 'Stops commands that inspect terminal history files such as `.zsh_history` and `.bash_history`.',
        category: 'Privacy',
        payload: {
            id: 'block-shell-history-read',
            name: 'Block shell history reads',
            enabled: true,
            kind: 'resource_access',
            group: '',
            scope: {
                scenarios: ['claude_code'],
            },
            verdict: 'block',
            reason: 'This policy blocks attempts to read shell history files.',
            match: {
                tool_names: ['bash'],
                actions: { include: ['read'] },
                resources: {
                    type: 'path',
                    mode: 'contains',
                    values: ['.zsh_history', '.bash_history'],
                },
            },
        },
    },
    {
        key: 'block-git-config-read',
        icon: <Rule sx={{ fontSize: 28, color: 'primary.main' }} />,
        title: 'Block Git credential config reads',
        description: 'Prevents reads of `.git-credentials` and related config files that may contain stored tokens.',
        category: 'Credentials',
        payload: {
            id: 'block-git-credentials-read',
            name: 'Block Git credential config reads',
            enabled: true,
            kind: 'resource_access',
            group: '',
            scope: {
                scenarios: ['claude_code'],
            },
            verdict: 'block',
            reason: 'This policy blocks attempts to read Git credential and configuration files that may contain secrets.',
            match: {
                tool_names: ['bash'],
                actions: { include: ['read'] },
                resources: {
                    type: 'path',
                    mode: 'contains',
                    values: ['.git-credentials', '.gitconfig'],
                },
            },
        },
    },
];

const GuardrailsMarketPage = () => {
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [existingPolicyIds, setExistingPolicyIds] = useState<string[]>([]);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [search, setSearch] = useState('');
    const [selectedCategory, setSelectedCategory] = useState<string>('All');

    const categories = useMemo(() => {
        return ['All', ...Array.from(new Set(ruleTemplates.map((template) => template.category)))];
    }, []);

    const filteredTemplates = useMemo(() => {
        const query = search.trim().toLowerCase();
        return ruleTemplates.filter((template) => {
            const matchesCategory = selectedCategory === 'All' || template.category === selectedCategory;
            const matchesSearch =
                query.length === 0
                || template.title.toLowerCase().includes(query)
                || template.description.toLowerCase().includes(query)
                || template.payload.id.toLowerCase().includes(query)
                || template.category.toLowerCase().includes(query);
            return matchesCategory && matchesSearch;
        });
    }, [search, selectedCategory]);

    const groupedTemplates = useMemo(() => {
        return filteredTemplates.reduce<Record<string, RuleTemplate[]>>((acc, template) => {
            if (!acc[template.category]) {
                acc[template.category] = [];
            }
            acc[template.category].push(template);
            return acc;
        }, {});
    }, [filteredTemplates]);

    const loadExistingPolicies = async () => {
        try {
            setLoading(true);
            const guardrailsConfig = await api.getGuardrailsConfig();
            const ids = (guardrailsConfig?.config?.policies || [])
                .map((policy: any) => policy?.id)
                .filter(Boolean);
            setExistingPolicyIds(ids);
        } catch (error) {
            console.error('Failed to load guardrails config for market:', error);
            setActionMessage({ type: 'error', text: 'Failed to load existing policies.' });
            setExistingPolicyIds([]);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadExistingPolicies();
    }, []);

    const buildUniquePolicyId = (baseId: string) => {
        const existing = new Set(existingPolicyIds);
        if (!existing.has(baseId)) {
            return baseId;
        }
        let suffix = 2;
        let nextId = `${baseId}-${suffix}`;
        while (existing.has(nextId)) {
            suffix += 1;
            nextId = `${baseId}-${suffix}`;
        }
        return nextId;
    };

    const slugify = (value: string) =>
        value
            .toLowerCase()
            .trim()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '');

    const makeDraftFromTemplate = (template: RuleTemplate) => {
        const payload = template.payload || {};
        const match = payload.match || {};
        const baseId = payload.id || slugify(payload.name || template.title || 'policy-template');
        return {
            id: buildUniquePolicyId(baseId),
            name: payload.name || template.title,
            group: payload.group || '',
            kind: payload.kind || 'resource_access',
            enabled: payload.enabled !== false,
            verdict: payload.verdict || 'block',
            scenarios: Array.isArray(payload.scope?.scenarios) ? payload.scope.scenarios : ['claude_code'],
            toolNames: Array.isArray(match.tool_names) ? match.tool_names.join('\n') : '',
            actions: Array.isArray(match.actions?.include) ? match.actions.include : [],
            commandTerms: Array.isArray(match.terms) ? match.terms.join('\n') : '',
            resources: Array.isArray(match.resources?.values) ? match.resources.values.join('\n') : '',
            resourceMode: match.resources?.mode || 'prefix',
            patterns: Array.isArray(match.patterns) ? match.patterns.join('\n') : '',
            patternMode: match.pattern_mode || 'substring',
            caseSensitive: !!match.case_sensitive,
            reason: payload.reason || '',
        };
    };

    const handleInstallTemplate = (template: RuleTemplate) => {
        navigate('/guardrails/rules', {
            state: {
                newPolicyDraft: makeDraftFromTemplate(template),
            },
        });
    };

    const formatKindLabel = (kind: string) => {
        switch (kind) {
            case 'resource_access':
                return 'Resource Access';
            case 'command_execution':
                return 'Command Execution';
            case 'content':
                return 'Content';
            default:
                return kind;
        }
    };

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Builtins"
                    subtitle="Start from curated Guardrails policy templates instead of creating every policy from scratch."
                    size="full"
                >
                    <Stack spacing={2}>
                        <Typography variant="body2" color="text.secondary">
                            These templates are local starters. Install opens the policy editor with a prefilled draft. Nothing is saved until you click Save there.
                        </Typography>
                        {actionMessage && (
                            <Alert severity={actionMessage.type} onClose={() => setActionMessage(null)}>
                                {actionMessage.text}
                            </Alert>
                        )}
                        <Stack direction={{ xs: 'column', lg: 'row' }} spacing={2} alignItems={{ lg: 'center' }}>
                            <TextField
                                size="small"
                                label="Search templates"
                                value={search}
                                onChange={(e) => setSearch(e.target.value)}
                                sx={{ minWidth: { lg: 280 } }}
                            />
                            <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap', rowGap: 1 }}>
                                {categories.map((category) => (
                                    <Chip
                                        key={category}
                                        label={category}
                                        clickable
                                        color={selectedCategory === category ? 'primary' : 'default'}
                                        variant={selectedCategory === category ? 'filled' : 'outlined'}
                                        onClick={() => setSelectedCategory(category)}
                                    />
                                ))}
                            </Stack>
                        </Stack>
                    </Stack>
                </UnifiedCard>

                {Object.keys(groupedTemplates).length === 0 && (
                    <UnifiedCard title="No matching templates" size="full">
                        <Typography variant="body2" color="text.secondary">
                            No local rule templates match the current filters.
                        </Typography>
                    </UnifiedCard>
                )}

                {Object.entries(groupedTemplates).map(([category, templates]) => (
                    <UnifiedCard key={category} title={category} size="full">
                        <Stack spacing={1.5}>
                            {templates.map((template) => (
                                <Box
                                    key={template.key}
                                    sx={{
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 2,
                                        px: 2,
                                        py: 1.5,
                                    }}
                                >
                                    <Stack
                                        direction={{ xs: 'column', lg: 'row' }}
                                        spacing={1.5}
                                        alignItems={{ lg: 'center' }}
                                        justifyContent="space-between"
                                    >
                                        <Stack direction="row" spacing={1.5} sx={{ minWidth: 0, flex: 1 }}>
                                            <Box
                                                sx={{
                                                    width: 40,
                                                    height: 40,
                                                    borderRadius: 2,
                                                    bgcolor: 'action.hover',
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    justifyContent: 'center',
                                                    flexShrink: 0,
                                                }}
                                            >
                                                {template.icon}
                                            </Box>
                                            <Stack spacing={0.75} sx={{ minWidth: 0, flex: 1 }}>
                                                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }} useFlexGap flexWrap="wrap">
                                                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                                        {template.title}
                                                    </Typography>
                                                    <Chip size="small" label={formatKindLabel(template.payload.kind)} variant="outlined" />
                                                    <Chip size="small" label={template.category} variant="outlined" />
                                                </Stack>
                                                <Typography variant="body2" color="text.secondary">
                                                    {template.description}
                                                </Typography>
                                            </Stack>
                                        </Stack>
                                        <Stack direction="row" spacing={1} sx={{ flexShrink: 0 }}>
                                            <Button
                                                variant="contained"
                                                startIcon={<AddShoppingCart />}
                                                onClick={() => handleInstallTemplate(template)}
                                            >
                                                Install
                                            </Button>
                                        </Stack>
                                    </Stack>
                                </Box>
                            ))}
                        </Stack>
                    </UnifiedCard>
                ))}
            </Stack>
        </PageLayout>
    );
};

export default GuardrailsMarketPage;
