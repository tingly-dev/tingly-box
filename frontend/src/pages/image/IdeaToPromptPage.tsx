import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Box, Button, ButtonBase, Skeleton, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { IconWriting } from '@tabler/icons-react';
import UnifiedCard from '@/components/UnifiedCard';
import { Palette, tablerMui } from '@/components/icons';
import { useNotify } from '@/hooks/useNotify';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement';
import ImageModelPicker from './components/ImageModelPicker';
import { imageModelOptions } from './components/imageModelRules';
import { saveSnippet } from './library/assetStore';
import { deriveLabel } from './profiles/promptLabel';
import InstructionsField from './prompt/InstructionsField';
import PromptRunCard from './prompt/PromptRunCard';
import { DEFAULT_IDEA_INSTRUCTIONS, IDEA_STARTERS, type IdeaHandoff } from './prompt/ideaPromptTemplates';
import {
    clearIdeaPromptRuns,
    removeIdeaPromptRun,
    startIdeaPromptRun,
    updateIdeaPromptRun,
    useIdeaPromptRuns,
    type IdeaPromptRun,
} from './prompt/ideaPromptStore';
import { useInstructions, useRememberedModel } from './prompt/instructionStarters';
import { PROMPT_TOOLS_SCENARIO as SCENARIO } from './prompt/promptRunStore';

const Writing = tablerMui(IconWriting);

const INSTRUCTIONS_KEY = 'image.idea.instructions';
// Its own memory, apart from Image to Prompt's: that page needs a model that
// can see, this one is fine with a cheaper text model from the same rules.
const MODEL_KEY = 'image.idea.model';
const WORKBENCH_MIN_HEIGHT = 600;

// Idea to Prompt (扩写提示词): a short description in, a full, editable
// prompt out, handed on to the Playground or the Library. Image to Prompt's
// sibling: the same workbench, instructions field, run cards and imageprompt
// scenario; only the input is text. See .design/idea-to-prompt.md.
const IdeaToPromptPage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const location = useLocation();
    const { notify } = useNotify();
    const showNotification = useCallback(
        (message: string, severity: 'success' | 'info' | 'warning' | 'error') => { notify(severity, message); },
        [notify],
    );

    const { rules, loadingRule, loadRules } = useRuleManagement();
    useEffect(() => { void loadRules(SCENARIO); }, [loadRules]);
    const reloadRules = useCallback(() => loadRules(SCENARIO), [loadRules]);
    const models = useMemo(() => imageModelOptions(rules).map((option) => option.requestModel), [rules]);
    const [model, chooseModel] = useRememberedModel(MODEL_KEY, models);
    const [instructions, changeInstructions] = useInstructions(INSTRUCTIONS_KEY, DEFAULT_IDEA_INSTRUCTIONS);

    // Arriving from the Playground's prompt box: its text is the description.
    const [handed] = useState(() => (location.state as IdeaHandoff | null)?.description?.trim() || '');
    const [description, setDescription] = useState(handed);

    const runs = useIdeaPromptRuns();
    const canRun = Boolean(description.trim()) && Boolean(model);
    const writePrompt = () => {
        if (!canRun) return;
        void startIdeaPromptRun({ description: description.trim(), model, instructions });
    };

    // The person already asked for it in the Playground, so the handed text
    // runs once the rules are in, if there is a model to run it on. The
    // history entry is cleared so a reload does not run it again.
    const handedRun = useRef(Boolean(handed));
    useEffect(() => {
        if (!handedRun.current || loadingRule) return;
        handedRun.current = false;
        navigate(location.pathname, { replace: true, state: null });
        if (model) void startIdeaPromptRun({ description: handed, model, instructions });
    }, [handed, instructions, loadingRule, location.pathname, model, navigate]);

    // Stable, so typing does not re-render every card.
    const latest = useRef({ models, changeInstructions, chooseModel });
    latest.current = { models, changeInstructions, chooseModel };
    const reuseRun = useCallback((run: IdeaPromptRun) => {
        setDescription(run.description);
        latest.current.changeInstructions(run.instructions);
        if (latest.current.models.includes(run.model)) latest.current.chooseModel(run.model);
    }, []);
    const usePrompt = useCallback((prompt: string) => navigate('/image/playground', { state: { prompt } }), [navigate]);
    const saveAsSnippet = useCallback((text: string) => {
        saveSnippet({ name: deriveLabel(text, 12) || t('ideaPrompt.title', { defaultValue: 'Idea to Prompt' }), text });
        showNotification(t('imagePrompt.savedSnippet', { defaultValue: 'Saved to Library snippets' }), 'success');
    }, [showNotification, t]);

    const submitOnShortcut = (event: React.KeyboardEvent) => {
        if ((event.metaKey || event.ctrlKey) && event.key === 'Enter' && canRun) {
            event.preventDefault();
            writePrompt();
        }
    };

    return (
        <Box sx={{ height: { lg: '100%' }, minHeight: { lg: WORKBENCH_MIN_HEIGHT } }}>
            <UnifiedCard
                size="full"
                titleHeadingLevel={1}
                sx={{ height: { lg: '100%' } }}
                title={t('ideaPrompt.title', { defaultValue: 'Idea to Prompt' })}
                subtitle={t('ideaPrompt.subtitle', { defaultValue: 'Write a few words and get a full prompt for an image model.' })}
                rightAction={(
                    <Button variant="outlined" size="small" startIcon={<Palette />} onClick={() => navigate('/image/playground')}>
                        {t('imageProfile.backToPlayground', { defaultValue: 'Open Playground' })}
                    </Button>
                )}
            >
                <Box
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: { xs: '1fr', lg: 'minmax(360px, 420px) minmax(0, 1fr)' },
                        gridTemplateRows: { lg: 'minmax(0, 1fr)' },
                        height: { lg: '100%' },
                        gap: 3,
                        alignItems: 'stretch',
                    }}
                >
                    <Stack spacing={2} sx={{ minWidth: 0, height: { lg: '100%' } }}>
                        <Box>
                            <Typography variant="caption" sx={{ display: 'block', mb: 0.5, color: 'text.secondary' }}>
                                {t('ideaPrompt.description', { defaultValue: 'Description' })}
                                {' · '}
                                {t('ideaPrompt.descriptionHint', { defaultValue: 'a few words, any language' })}
                            </Typography>
                            <TextField
                                multiline
                                fullWidth
                                autoFocus={!handed}
                                value={description}
                                onChange={(event) => setDescription(event.target.value)}
                                onKeyDown={submitOnShortcut}
                                minRows={4}
                                maxRows={8}
                                placeholder={t('ideaPrompt.descriptionPlaceholder', { defaultValue: 'e.g. a cat on the Moon looking at the Earth' })}
                                slotProps={{ htmlInput: { 'aria-label': t('ideaPrompt.description', { defaultValue: 'Description' }) } }}
                                sx={{ '& .MuiInputBase-root': { fontSize: 14, lineHeight: 1.55 } }}
                            />
                        </Box>

                        {loadingRule ? (
                            <Skeleton variant="rounded" height={40} />
                        ) : (
                            <ImageModelPicker
                                scenario={SCENARIO}
                                label={t('ideaPrompt.model', { defaultValue: 'Model' })}
                                chooserTitle={t('ideaPrompt.chooseModel', { defaultValue: 'Choose a chat model to write the prompt' })}
                                rules={rules}
                                value={model}
                                onChange={chooseModel}
                                onRulesReload={reloadRules}
                                showNotification={showNotification}
                            />
                        )}

                        <InstructionsField
                            value={instructions}
                            onChange={changeInstructions}
                            starters={IDEA_STARTERS}
                            defaultText={DEFAULT_IDEA_INSTRUCTIONS}
                            i18nGroup="ideaPrompt"
                            onSubmit={writePrompt}
                            canSubmit={canRun}
                        />

                        <Tooltip title={t('ideaPrompt.submitShortcut', { defaultValue: '⌘/Ctrl + Enter in the description or the instructions' })} placement="top">
                            <span>
                                <Button variant="contained" fullWidth disabled={!canRun} onClick={writePrompt} startIcon={<Writing />}>
                                    {t('ideaPrompt.run', { defaultValue: 'Write prompt' })}
                                </Button>
                            </span>
                        </Tooltip>
                    </Stack>

                    <Box
                        sx={{
                            minWidth: 0,
                            height: { lg: '100%' },
                            display: 'flex',
                            flexDirection: 'column',
                            border: 1,
                            borderColor: 'divider',
                            borderRadius: 2,
                            overflow: 'hidden',
                        }}
                    >
                        <Stack direction="row" sx={{ alignItems: 'center', px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}>
                            <Typography variant="subtitle2" sx={{ mr: 'auto' }}>
                                {t('imagePrompt.results', { defaultValue: 'Prompts' })}
                            </Typography>
                            {runs.length > 0 && (
                                <Button size="small" color="inherit" onClick={clearIdeaPromptRuns} sx={{ color: 'text.secondary' }}>
                                    {t('imagePrompt.clear', { defaultValue: 'Clear' })}
                                </Button>
                            )}
                        </Stack>
                        <Box sx={{ flex: 1, minHeight: { xs: 240, lg: 0 }, overflow: 'auto', p: 2 }}>
                            {runs.length === 0 ? (
                                <Stack sx={{ height: '100%', minHeight: 200, alignItems: 'center', justifyContent: 'center', textAlign: 'center', color: 'text.secondary', px: 4 }}>
                                    <Typography variant="body2">
                                        {t('ideaPrompt.empty', { defaultValue: 'Prompts you write appear here.' })}
                                    </Typography>
                                    <Typography variant="caption" sx={{ mt: 0.5, color: 'text.disabled', maxWidth: 420 }}>
                                        {t('ideaPrompt.emptyHint', { defaultValue: 'Edit one, then use it in the Playground or keep it as a snippet. “Write a full prompt from this” in the Playground prompt box brings its text here too.' })}
                                    </Typography>
                                </Stack>
                            ) : (
                                <Stack spacing={1.5}>
                                    {runs.map((run) => (
                                        <IdeaPromptRunCard
                                            key={run.id}
                                            run={run}
                                            onReuse={reuseRun}
                                            onUse={usePrompt}
                                            onSaveSnippet={saveAsSnippet}
                                        />
                                    ))}
                                </Stack>
                            )}
                        </Box>
                    </Box>
                </Box>
            </UnifiedCard>
        </Box>
    );
};

interface IdeaPromptRunCardProps {
    run: IdeaPromptRun;
    onReuse: (run: IdeaPromptRun) => void;
    onUse: (prompt: string) => void;
    onSaveSnippet: (prompt: string) => void;
}

const commitPrompt = (id: string, prompt: string) => updateIdeaPromptRun(id, { prompt });

// One run: the description it started from above the prompt. Memoised: the
// page re-renders on every keystroke, and the list keeps runs across days.
const IdeaPromptRunCard = memo(({ run, onReuse, onUse, onSaveSnippet }: IdeaPromptRunCardProps) => {
    const { t } = useTranslation();
    const reuseLabel = t('ideaPrompt.reuse', { defaultValue: 'Put this description and its instructions back in the panel' });
    return (
        <PromptRunCard
            run={run}
            header={(
                <Tooltip title={reuseLabel} placement="top-start">
                    <ButtonBase
                        onClick={() => onReuse(run)}
                        aria-label={reuseLabel}
                        sx={{
                            justifyContent: 'flex-start',
                            textAlign: 'left',
                            alignSelf: 'flex-start',
                            maxWidth: '100%',
                            px: 1,
                            py: 0.25,
                            borderRadius: '0 4px 4px 0',
                            borderLeft: 2,
                            borderColor: 'divider',
                            color: 'text.secondary',
                            '&:hover': { bgcolor: 'action.hover', color: 'text.primary' },
                        }}
                    >
                        <Typography variant="body2" noWrap sx={{ color: 'inherit' }}>{run.description}</Typography>
                    </ButtonBase>
                </Tooltip>
            )}
            pendingLabel={t('ideaPrompt.pending', { defaultValue: 'Writing the prompt…' })}
            failedHint={t('ideaPrompt.failedHint', { defaultValue: 'Retry, or choose another model.' })}
            onCommit={commitPrompt}
            onUse={onUse}
            onSaveSnippet={onSaveSnippet}
            onRetry={() => { void startIdeaPromptRun(run, run.id); }}
            onRemove={removeIdeaPromptRun}
        />
    );
});
IdeaPromptRunCard.displayName = 'IdeaPromptRunCard';

export default IdeaToPromptPage;
