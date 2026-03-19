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
    ArticleOutlined,
    CallMade,
    Folder,
    RestartAlt,
    Rule,
    Shield,
    Terminal,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

type BuiltinTemplate = {
    id: string;
    name: string;
    summary?: string;
    description?: string;
    kind: 'resource_access' | 'command_execution' | 'content';
    topic?: string;
    tags?: string[];
    policy: any;
};

const GuardrailsMarketPage = () => {
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [existingPolicyIds, setExistingPolicyIds] = useState<string[]>([]);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [search, setSearch] = useState('');
    const [selectedTopics, setSelectedTopics] = useState<string[]>([]);
    const [templates, setTemplates] = useState<BuiltinTemplate[]>([]);

    const topicOptions = useMemo(() => {
        const topics = templates.map((template) => template.topic).filter(Boolean) as string[];
        return Array.from(new Set(topics)).sort((a, b) => a.localeCompare(b));
    }, [templates]);

    const filteredTemplates = useMemo(() => {
        const query = search.trim().toLowerCase();
        return templates.filter((template) => {
            const matchesTopic = selectedTopics.length === 0 || (template.topic ? selectedTopics.includes(template.topic) : false);
            const haystack = [
                template.name,
                template.summary,
                template.description,
                template.topic,
                template.kind,
                ...(template.tags || []),
            ]
                .filter(Boolean)
                .join(' ')
                .toLowerCase();
            const matchesSearch = query.length === 0 || haystack.includes(query);
            return matchesTopic && matchesSearch;
        });
    }, [search, selectedTopics, templates]);

    const groupedTemplates = useMemo(() => {
        return filteredTemplates.reduce<Record<string, BuiltinTemplate[]>>((acc, template) => {
            const topic = template.topic || 'Uncategorized';
            if (!acc[topic]) {
                acc[topic] = [];
            }
            acc[topic].push(template);
            return acc;
        }, {});
    }, [filteredTemplates]);

    useEffect(() => {
        const loadBuiltins = async () => {
            try {
                setLoading(true);
                const [response, configResponse] = await Promise.all([
                    api.getGuardrailsBuiltins(),
                    api.getGuardrailsConfig(),
                ]);
                const nextTemplates = Array.isArray(response?.templates) ? response.templates : [];
                const nextPolicyIDs = Array.isArray(configResponse?.config?.policies)
                    ? configResponse.config.policies.map((policy: any) => policy?.id).filter(Boolean)
                    : [];
                setTemplates(nextTemplates);
                setExistingPolicyIds(nextPolicyIDs);
                setActionMessage(null);
            } catch (error: any) {
                console.error('Failed to load guardrails builtins:', error);
                setTemplates([]);
                setExistingPolicyIds([]);
                setActionMessage({ type: 'error', text: error?.message || 'Failed to load builtin policies.' });
            } finally {
                setLoading(false);
            }
        };

        loadBuiltins();
    }, []);

    const slugify = (value: string) =>
        value
            .toLowerCase()
            .trim()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '');

    const buildUniquePolicyID = (baseID: string) => {
        const existing = new Set(existingPolicyIds);
        if (!existing.has(baseID)) {
            return baseID;
        }
        let suffix = 2;
        let nextID = `${baseID}-${suffix}`;
        while (existing.has(nextID)) {
            suffix += 1;
            nextID = `${baseID}-${suffix}`;
        }
        return nextID;
    };

    const normalizeGroup = (value?: string) => value?.trim() || '';

    const buildDraftFromTemplate = (template: BuiltinTemplate) => {
        const payload = template.policy || {};
        const match = payload.match || {};
        const baseID = payload.id || template.id || slugify(template.name || 'policy-template');
        return {
            id: buildUniquePolicyID(baseID),
            name: payload.name || template.name,
            group: normalizeGroup(payload.group),
            kind: payload.kind || template.kind,
            enabled: payload.enabled !== false,
            verdict: payload.verdict || 'block',
            scenarios: Array.isArray(payload.scope?.scenarios) ? payload.scope.scenarios : [],
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

    const handleInstallTemplate = (template: BuiltinTemplate) => {
        navigate('/guardrails/rules', {
            state: {
                newPolicyDraft: buildDraftFromTemplate(template),
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

    const formatTopicFilterLabel = (topic?: string) => {
        switch (topic) {
            case 'filesystem_access':
                return 'Filesystem Access';
            case 'command_execution':
                return 'Command Execution';
            case 'output_filtering':
                return 'Output Protection';
            default:
                if (!topic) {
                    return 'Uncategorized';
                }
                return topic
                    .split(/[_-]/)
                    .filter(Boolean)
                    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
                    .join(' ');
        }
    };

    const formatTopicLabel = (topic?: string) => {
        switch (topic) {
            case 'filesystem_access':
                return 'Filesystem Access Control';
            case 'command_execution':
                return 'Command Execution';
            case 'output_filtering':
                return 'Sensitive Output Protection';
            default:
                return formatTopicFilterLabel(topic);
        }
    };

    const formatTopicDescription = (topic?: string) => {
        switch (topic) {
            case 'filesystem_access':
                return 'Policies for protected files, directories, shell history, credentials, and other local resources.';
            case 'command_execution':
                return 'Policies for risky shell execution, download-and-exec patterns, and outbound command flows.';
            case 'output_filtering':
                return 'Policies for filtering sensitive content from model output or tool results.';
            default:
                return 'Builtin policies grouped by topic.';
        }
    };

    const getTemplateIcon = (template: BuiltinTemplate): ReactNode => {
        switch (template.kind) {
            case 'resource_access':
                if (template.topic === 'filesystem_access') return <Folder sx={{ fontSize: 24, color: 'primary.main' }} />;
                return <Shield sx={{ fontSize: 24, color: 'primary.main' }} />;
            case 'command_execution':
                if (template.topic === 'command_execution') return <Terminal sx={{ fontSize: 24, color: 'primary.main' }} />;
                return <CallMade sx={{ fontSize: 24, color: 'primary.main' }} />;
            case 'content':
                return <ArticleOutlined sx={{ fontSize: 24, color: 'primary.main' }} />;
            default:
                return <Shield sx={{ fontSize: 24, color: 'primary.main' }} />;
        }
    };

    const hasActiveFilters = search.trim().length > 0 || selectedTopics.length > 0;

    const toggleMultiFilter = (value: string, selected: string[], setSelected: (values: string[]) => void) => {
        setSelected(selected.includes(value) ? selected.filter((item) => item !== value) : [...selected, value]);
    };

    const resetFilters = () => {
        setSearch('');
        setSelectedTopics([]);
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
                            Builtins are local starter policies. Install opens the main policy editor with a prefilled draft. Nothing is saved until you click Save there.
                        </Typography>
                        {actionMessage && (
                            <Alert severity={actionMessage.type} onClose={() => setActionMessage(null)}>
                                {actionMessage.text}
                            </Alert>
                        )}
                        <Box sx={{ px: 0.5 }}>
                            <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} alignItems={{ md: 'center' }}>
                                <TextField
                                    size="small"
                                    label="Search builtins"
                                    value={search}
                                    onChange={(e) => setSearch(e.target.value)}
                                    sx={{ flex: 1 }}
                                />
                                <Stack direction="row" spacing={1} alignItems="center" sx={{ flexShrink: 0 }}>
                                    <Chip size="small" label={`${filteredTemplates.length} results`} variant="outlined" />
                                    <Button
                                        size="small"
                                        variant="text"
                                        startIcon={<RestartAlt fontSize="small" />}
                                        disabled={!hasActiveFilters}
                                        onClick={resetFilters}
                                    >
                                        Reset
                                    </Button>
                                </Stack>
                            </Stack>

                            <Stack spacing={1.25} sx={{ mt: 1.5 }}>
                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} alignItems={{ md: 'center' }}>
                                    <Typography variant="caption" color="text.secondary" sx={{ minWidth: { md: 64 } }}>
                                        Category
                                    </Typography>
                                    <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap', rowGap: 1 }}>
                                        <Chip
                                            label="All"
                                            clickable
                                            color={selectedTopics.length === 0 ? 'primary' : 'default'}
                                            variant={selectedTopics.length === 0 ? 'filled' : 'outlined'}
                                            onClick={() => setSelectedTopics([])}
                                        />
                                        {topicOptions.map((topic) => (
                                            <Chip
                                                key={topic}
                                                label={formatTopicFilterLabel(topic)}
                                                clickable
                                                color={selectedTopics.includes(topic) ? 'primary' : 'default'}
                                                variant={selectedTopics.includes(topic) ? 'filled' : 'outlined'}
                                                onClick={() => toggleMultiFilter(topic, selectedTopics, setSelectedTopics)}
                                            />
                                        ))}
                                    </Stack>
                                </Stack>

                            </Stack>
                        </Box>
                    </Stack>
                </UnifiedCard>

                {Object.keys(groupedTemplates).length === 0 && (
                    <UnifiedCard title="No matching builtins" size="full">
                        <Typography variant="body2" color="text.secondary">
                            No builtin policies match the current filters.
                        </Typography>
                    </UnifiedCard>
                )}

                {Object.entries(groupedTemplates).map(([topic, items]) => (
                    <UnifiedCard key={topic} title={formatTopicLabel(topic)} subtitle={formatTopicDescription(topic)} size="full">
                        <Stack spacing={1.25}>
                            {items.map((template) => (
                                <Box
                                    key={template.id}
                                    sx={{
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 2,
                                        overflow: 'hidden',
                                        bgcolor: 'background.paper',
                                    }}
                                >
                                    <Box
                                        sx={{
                                            px: 2,
                                            py: 1,
                                            borderBottom: '1px solid',
                                            borderColor: 'divider',
                                            bgcolor: 'action.hover',
                                        }}
                                    >
                                        <Stack
                                            direction={{ xs: 'column', sm: 'row' }}
                                            spacing={1}
                                            alignItems={{ sm: 'center' }}
                                            justifyContent="space-between"
                                        >
                                            <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                                <Chip size="small" label={formatKindLabel(template.kind)} variant="outlined" />
                                            </Stack>
                                            <Button
                                                variant="contained"
                                                size="small"
                                                startIcon={<AddShoppingCart />}
                                                onClick={() => handleInstallTemplate(template)}
                                            >
                                                Install
                                            </Button>
                                        </Stack>
                                    </Box>
                                    <Stack
                                        direction="row"
                                        spacing={1.5}
                                        sx={{ px: 2, py: 1.5 }}
                                    >
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
                                            {getTemplateIcon(template)}
                                        </Box>
                                        <Stack spacing={0.75} sx={{ minWidth: 0, flex: 1 }}>
                                            <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                                {template.name}
                                            </Typography>
                                            <Typography variant="body2" color="text.secondary">
                                                {template.summary || template.description}
                                            </Typography>
                                            {(template.tags?.length ?? 0) > 0 && (
                                                <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
                                                    {(template.tags || []).slice(0, 4).map((tag) => (
                                                        <Chip key={tag} size="small" label={tag} variant="outlined" />
                                                    ))}
                                                </Stack>
                                            )}
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
