import React, { useState } from 'react';
import { Box, Button, Chip, IconButton, ListItemText, Menu, MenuItem, Paper, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { Close as RemoveIcon, KeyboardArrowUp as UpIcon, KeyboardArrowDown as DownIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import type { ProbeMessageRole } from '@/types/probe';
import type { PlaygroundTurn } from './playgroundState';

// ConversationEditor: system prompt + the turn list the probe sends. Many
// flags only act on particular message shapes (a mid-conversation system
// turn, a tool round-trip), so the editor ships those shapes as templates —
// the harness fixture knowledge, surfaced (.design/playground.md §6).

const ROLES: ProbeMessageRole[] = ['user', 'assistant', 'system'];
const ROLE_COLOR: Record<ProbeMessageRole, 'primary' | 'success' | 'warning'> = { user: 'primary', assistant: 'success', system: 'warning' };

export interface ConversationTemplate {
    id: 'multi' | 'midsys' | 'tool';
    turns: PlaygroundTurn[];
    // Templates that only make sense with the Tool axis on say so, and the
    // page flips the axis when one is inserted.
    tool?: boolean;
}

export const TEMPLATES: ConversationTemplate[] = [
    {
        id: 'multi',
        turns: [
            { role: 'user', text: "What's in ./src?" },
            { role: 'assistant', text: 'Three files: main.go, helper.go, types.go.' },
            { role: 'user', text: 'Summarize helper.go in one line.' },
        ],
    },
    {
        id: 'midsys',
        turns: [
            { role: 'user', text: "What's in ./src?" },
            { role: 'assistant', text: 'Three files: main.go, helper.go, types.go.' },
            { role: 'system', text: 'From now on answer only in JSON.' },
            { role: 'user', text: 'List them again.' },
        ],
    },
    {
        id: 'tool',
        tool: true,
        turns: [
            { role: 'user', text: "Please use the bash tool to list the current directory contents with 'ls -la'." },
            { role: 'assistant', text: 'I will run `ls -la` with the bash tool.' },
            { role: 'user', text: 'Tool result: total 3\nmain.go helper.go types.go — now summarize.' },
        ],
    },
];

export const ConversationEditor: React.FC<{
    system: string;
    onSystemChange: (system: string) => void;
    turns: PlaygroundTurn[];
    onTurnsChange: (turns: PlaygroundTurn[]) => void;
    onTemplate?: (template: ConversationTemplate) => void;
    visionOn: boolean;
}> = ({ system, onSystemChange, turns, onTurnsChange, onTemplate, visionOn }) => {
    const { t } = useTranslation();
    const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);

    const update = (i: number, patch: Partial<PlaygroundTurn>) =>
        onTurnsChange(turns.map((turn, j) => (j === i ? { ...turn, ...patch } : turn)));
    const move = (i: number, dir: -1 | 1) => {
        const j = i + dir;
        if (j < 0 || j >= turns.length) return;
        const next = [...turns];
        [next[i], next[j]] = [next[j], next[i]];
        onTurnsChange(next);
    };
    const remove = (i: number) => onTurnsChange(turns.filter((_, j) => j !== i));
    const lastIsUser = turns.length === 0 || turns[turns.length - 1].role === 'user';

    return (
        <Stack spacing={1.5}>
            <TextField
                label={t('playground.system', { defaultValue: 'System' })}
                size="small"
                multiline
                minRows={1}
                maxRows={5}
                value={system}
                onChange={(e) => onSystemChange(e.target.value)}
                placeholder={t('playground.systemPlaceholder', { defaultValue: "Empty = the probe's default echo instruction" })}
                slotProps={{ htmlInput: { sx: { fontSize: '0.82rem' } }, inputLabel: { shrink: true } }}
            />

            {turns.length === 0 && (
                <Typography variant="caption" sx={{ color: 'text.secondary', lineHeight: 1.4 }}>
                    {t('playground.noTurns', { defaultValue: 'No turns: the probe sends its built-in fixture message (tool-aware). Add a turn to write your own conversation.' })}
                </Typography>
            )}

            {turns.map((turn, i) => (
                <Paper key={i} variant="outlined" sx={{ overflow: 'hidden' }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, px: 1, py: 0.5, bgcolor: 'action.hover', borderBottom: '1px solid', borderColor: 'divider' }}>
                        <Tooltip title={t('playground.cycleRole', { defaultValue: 'Click to change the role' })}>
                            <Chip
                                size="small"
                                color={ROLE_COLOR[turn.role]}
                                variant="outlined"
                                label={turn.role}
                                onClick={() => update(i, { role: ROLES[(ROLES.indexOf(turn.role) + 1) % ROLES.length] })}
                                sx={{ height: 20, fontSize: '0.62rem', textTransform: 'uppercase', letterSpacing: '0.05em', fontWeight: 600 }}
                            />
                        </Tooltip>
                        {turn.role === 'system' && (
                            <Typography variant="caption" sx={{ color: 'text.disabled', fontSize: '0.62rem' }}>
                                {t('playground.midSystemHint', { defaultValue: 'mid-conversation system — non-standard on purpose' })}
                            </Typography>
                        )}
                        <Box sx={{ flex: 1 }} />
                        <Tooltip title={t('playground.moveUp', { defaultValue: 'Move up' })}>
                            <span><IconButton size="small" onClick={() => move(i, -1)} disabled={i === 0} sx={{ p: 0.25 }}><UpIcon sx={{ fontSize: 16 }} /></IconButton></span>
                        </Tooltip>
                        <Tooltip title={t('playground.moveDown', { defaultValue: 'Move down' })}>
                            <span><IconButton size="small" onClick={() => move(i, 1)} disabled={i === turns.length - 1} sx={{ p: 0.25 }}><DownIcon sx={{ fontSize: 16 }} /></IconButton></span>
                        </Tooltip>
                        <Tooltip title={t('playground.removeTurn', { defaultValue: 'Remove turn' })}>
                            <IconButton size="small" onClick={() => remove(i)} sx={{ p: 0.25 }}><RemoveIcon sx={{ fontSize: 16 }} /></IconButton>
                        </Tooltip>
                    </Box>
                    <TextField
                        fullWidth
                        multiline
                        minRows={2}
                        maxRows={8}
                        value={turn.text}
                        onChange={(e) => update(i, { text: e.target.value })}
                        placeholder={t('playground.turnPlaceholder', { defaultValue: 'Text of this turn' })}
                        variant="standard"
                        slotProps={{ input: { disableUnderline: true, sx: { px: 1.25, py: 0.75, fontSize: '0.82rem' } } }}
                    />
                </Paper>
            ))}

            {!lastIsUser && (
                <Typography variant="caption" sx={{ color: 'warning.main' }}>
                    {t('playground.lastTurnUser', { defaultValue: 'The last turn must be a user turn, or the probe is rejected.' })}
                </Typography>
            )}
            {visionOn && turns.length > 0 && (
                <Typography variant="caption" sx={{ color: 'warning.main' }}>
                    {t('playground.visionConflict', { defaultValue: 'The Vision axis brings its own fixture turn — turn it off to send a custom conversation.' })}
                </Typography>
            )}

            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Button size="small" variant="outlined" onClick={() => onTurnsChange([...turns, { role: 'user', text: '' }])}>
                    {t('playground.addTurn', { defaultValue: '+ turn' })}
                </Button>
                <Box sx={{ flex: 1 }} />
                <Button size="small" variant="outlined" onClick={(e) => setMenuAnchor(e.currentTarget)}>
                    {t('playground.templates', { defaultValue: 'Templates' })} ▾
                </Button>
                <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={() => setMenuAnchor(null)}>
                    {TEMPLATES.map((tpl) => (
                        <MenuItem
                            key={tpl.id}
                            onClick={() => {
                                setMenuAnchor(null);
                                onTurnsChange(tpl.turns.map((x) => ({ ...x })));
                                onTemplate?.(tpl);
                            }}
                            sx={{ maxWidth: 360, whiteSpace: 'normal' }}
                        >
                            <ListItemText
                                primary={t(`playground.template.${tpl.id}`)}
                                secondary={t(`playground.template.${tpl.id}Desc`)}
                                slotProps={{ primary: { sx: { fontSize: '0.85rem', fontWeight: 600 } }, secondary: { sx: { fontSize: '0.72rem' } } }}
                            />
                        </MenuItem>
                    ))}
                </Menu>
            </Box>
        </Stack>
    );
};

export default ConversationEditor;
