import { useEffect, useMemo, useState, type ReactNode } from 'react';
import {
    Alert,
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
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
            type: 'command_policy',
            enabled: true,
            scope: {
                scenarios: ['claude_code'],
                directions: ['response'],
                content_types: ['command'],
            },
            params: {
                kinds: ['shell'],
                actions: ['read'],
                resources: ['~/.ssh', '/etc/ssh'],
                resource_match: 'prefix',
                verdict: 'block',
                reason: 'This rule blocks attempts to read SSH configuration and key directories.',
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
            type: 'command_policy',
            enabled: true,
            scope: {
                scenarios: ['claude_code'],
                directions: ['response'],
                content_types: ['command'],
            },
            params: {
                kinds: ['shell'],
                actions: ['read'],
                resources: ['.env', '.env.local', '.env.production'],
                resource_match: 'contains',
                verdict: 'block',
                reason: 'This rule blocks attempts to read environment variable files that may contain secrets.',
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
            type: 'command_policy',
            enabled: true,
            scope: {
                scenarios: ['claude_code'],
                directions: ['response'],
                content_types: ['command'],
            },
            params: {
                kinds: ['shell'],
                actions: ['read'],
                resources: ['.zsh_history', '.bash_history'],
                resource_match: 'contains',
                verdict: 'block',
                reason: 'This rule blocks attempts to read shell history files.',
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
            type: 'command_policy',
            enabled: true,
            scope: {
                scenarios: ['claude_code'],
                directions: ['response'],
                content_types: ['command'],
            },
            params: {
                kinds: ['shell'],
                actions: ['read'],
                resources: ['.git-credentials', '.gitconfig'],
                resource_match: 'contains',
                verdict: 'block',
                reason: 'This rule blocks attempts to read Git credential and configuration files that may contain secrets.',
            },
        },
    },
];

const GuardrailsMarketPage = () => {
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [existingRuleIds, setExistingRuleIds] = useState<string[]>([]);
    const [pendingTemplateKey, setPendingTemplateKey] = useState<string | null>(null);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [search, setSearch] = useState('');
    const [selectedCategory, setSelectedCategory] = useState<string>('All');
    const [previewTemplate, setPreviewTemplate] = useState<RuleTemplate | null>(null);

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

    const loadExistingRules = async () => {
        try {
            setLoading(true);
            const guardrailsConfig = await api.getGuardrailsConfig();
            const ids = (guardrailsConfig?.config?.rules || [])
                .map((rule: any) => rule?.id)
                .filter(Boolean);
            setExistingRuleIds(ids);
        } catch (error) {
            console.error('Failed to load guardrails config for market:', error);
            setActionMessage({ type: 'error', text: 'Failed to load existing rules.' });
            setExistingRuleIds([]);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadExistingRules();
    }, []);

    const buildUniqueRuleId = (baseId: string) => {
        const existing = new Set(existingRuleIds);
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

    const handleInstallTemplate = async (template: RuleTemplate) => {
        try {
            setPendingTemplateKey(template.key);
            const nextId = buildUniqueRuleId(template.payload.id);
            const payload = {
                ...template.payload,
                id: nextId,
                name: nextId === template.payload.id ? template.payload.name : `${template.payload.name} (${nextId})`,
            };
            const result = await api.createGuardrailsRule(payload);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to install rule template.' });
                return;
            }
            setActionMessage({ type: 'success', text: `Installed rule template as "${nextId}".` });
            await loadExistingRules();
            navigate(`/guardrails/rules?ruleId=${encodeURIComponent(nextId)}`);
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to install rule template.' });
        } finally {
            setPendingTemplateKey(null);
        }
    };

    const formatTemplatePreview = (template: RuleTemplate) => {
        return JSON.stringify(template.payload, null, 2);
    };

    const buildTemplateSummary = (template: RuleTemplate) => {
        const params = template.payload?.params || {};
        const actions = Array.isArray(params.actions) ? params.actions : [];
        const resources = Array.isArray(params.resources) ? params.resources : [];
        return {
            actions: actions.length > 0 ? actions.join(', ') : 'any action',
            resources: resources.length > 0 ? resources.join(', ') : 'any resource',
            matchMode: params.resource_match || 'prefix',
        };
    };

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Rule Market"
                    subtitle="Start from curated Guardrails templates instead of creating every rule from scratch."
                    size="full"
                >
                    <Stack spacing={2}>
                        <Typography variant="body2" color="text.secondary">
                            These templates are local starter rules. Installing one writes it into your current Guardrails config and reloads the engine through the existing rule API.
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
                                        p: 2,
                                    }}
                                >
                                    <Stack
                                        direction={{ xs: 'column', lg: 'row' }}
                                        spacing={2}
                                        alignItems={{ lg: 'center' }}
                                        justifyContent="space-between"
                                    >
                                        <Stack direction="row" spacing={1.5} sx={{ minWidth: 0, flex: 1 }}>
                                            <Box
                                                sx={{
                                                    width: 48,
                                                    height: 48,
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
                                            <Stack spacing={1} sx={{ minWidth: 0, flex: 1 }}>
                                                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
                                                    <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                                                        {template.title}
                                                    </Typography>
                                                    <Chip size="small" label={template.payload.type} variant="outlined" />
                                                </Stack>
                                                <Typography variant="body2" color="text.secondary">
                                                    {template.description}
                                                </Typography>
                                                {(() => {
                                                    const summary = buildTemplateSummary(template);
                                                    return (
                                                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} useFlexGap flexWrap="wrap">
                                                            <Chip size="small" label={`Actions: ${summary.actions}`} />
                                                            <Chip size="small" label={`Resources: ${summary.resources}`} />
                                                            <Chip size="small" label={`Match: ${summary.matchMode}`} variant="outlined" />
                                                        </Stack>
                                                    );
                                                })()}
                                            </Stack>
                                        </Stack>
                                        <Stack
                                            direction={{ xs: 'row', lg: 'column' }}
                                            spacing={1}
                                            sx={{ flexShrink: 0, minWidth: { lg: 148 } }}
                                        >
                                            <Button
                                                variant="contained"
                                                startIcon={<AddShoppingCart />}
                                                disabled={pendingTemplateKey === template.key}
                                                onClick={() => handleInstallTemplate(template)}
                                            >
                                                {pendingTemplateKey === template.key ? 'Installing…' : 'Install'}
                                            </Button>
                                            <Button
                                                variant="outlined"
                                                onClick={() => setPreviewTemplate(template)}
                                            >
                                                Preview
                                            </Button>
                                        </Stack>
                                    </Stack>
                                </Box>
                            ))}
                        </Stack>
                    </UnifiedCard>
                ))}

                <Dialog
                    open={!!previewTemplate}
                    onClose={() => setPreviewTemplate(null)}
                    maxWidth="md"
                    fullWidth
                >
                    <DialogTitle>{previewTemplate?.title ?? 'Rule Preview'}</DialogTitle>
                    <DialogContent dividers>
                        {previewTemplate && (
                            <Box
                                component="pre"
                                sx={{
                                    m: 0,
                                    p: 2,
                                    borderRadius: 1.5,
                                    bgcolor: 'action.hover',
                                    overflowX: 'auto',
                                    fontSize: 13,
                                    lineHeight: 1.6,
                                    fontFamily: '"Fira Code", "Monaco", "Consolas", monospace',
                                }}
                            >
                                {formatTemplatePreview(previewTemplate)}
                            </Box>
                        )}
                    </DialogContent>
                    <DialogActions>
                        <Button onClick={() => setPreviewTemplate(null)}>Close</Button>
                        {previewTemplate && (
                            <Button
                                variant="contained"
                                startIcon={<AddShoppingCart />}
                                disabled={pendingTemplateKey === previewTemplate.key}
                                onClick={() => handleInstallTemplate(previewTemplate)}
                            >
                                {pendingTemplateKey === previewTemplate.key ? 'Installing…' : 'Install Template'}
                            </Button>
                        )}
                    </DialogActions>
                </Dialog>
            </Stack>
        </PageLayout>
    );
};

export default GuardrailsMarketPage;
