import {
    IconUser,
    IconYinYang,
    IconDots,
    IconLanguage,
    IconWand,
    IconMessageReport,
} from '@tabler/icons-react';
import { Box, Divider, ListItemButton, ListItemIcon, Menu, MenuItem, Tooltip, Typography } from '@mui/material';
import React, { useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import { Claude, Codex, OpenCode, Xcode, VSCode, OpenAI, Anthropic, OpenClaw, ClaudeDesktop } from '@/components/BrandIcons';
import {
    activityBarWidth,
    activityContainerPaddingY,
    activityItemPaddingX,
    activityItemRadius,
    activityItemSx,
    footerHeight,
    headerHeight,
} from './constants';
import type { ActivityItem } from './types';

interface ActivityBarProps {
    activityItems: ActivityItem[];
    activeActivity: string;
    onActivityClick: (item: ActivityItem) => void;
    onUserClick: (event: React.MouseEvent<HTMLElement>) => void;
    onZenToggle?: () => void;
    zenEnabled?: boolean;
    onMoreClick?: (event: React.MouseEvent<HTMLElement>) => void;
}

export const ZenActivityBar: React.FC<ActivityBarProps> = ({
    activityItems,
    activeActivity,
    onActivityClick,
    onUserClick,
    onZenToggle,
    zenEnabled = false,
    onMoreClick,
}) => {
    const { t, i18n } = useTranslation();
    const { currentVersion } = useAppVersion();
    const [zenMenuAnchorEl, setZenMenuAnchorEl] = useState<HTMLElement | null>(null);
    const [languageMenuAnchorEl, setLanguageMenuAnchorEl] = useState<HTMLElement | null>(null);

    const handleLanguageMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        setLanguageMenuAnchorEl(event.currentTarget);
    };

    const handleLanguageMenuClose = () => {
        setLanguageMenuAnchorEl(null);
    };

    const handleLanguageChange = (lng: string) => {
        i18n.changeLanguage(lng);
        localStorage.setItem('i18nextLng', lng);
        handleLanguageMenuClose();
    };

    const handleZenMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        setZenMenuAnchorEl(event.currentTarget);
    };

    const handleZenMenuClose = () => {
        setZenMenuAnchorEl(null);
    };

    const handleZenAgentSelect = (zenPath: string) => {
        // Set zen mode flag with the selected agent
        const agent = zenPath.replace('/zen/', '').replace('-', '_');
        localStorage.setItem('mock-flag-_global-zen', agent);
        // Reload page to activate zen mode
        window.location.href = zenPath;
    };

    return (
        <Box
            sx={{
                width: activityBarWidth,
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                bgcolor: 'background.paper',
                borderRight: '1px solid',
                borderColor: 'divider',
            }}
        >
            {/* Logo */}
            <Box
                sx={{
                    height: headerHeight,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                }}
            >
                <Tooltip title={`Tingly-Box v${currentVersion}`} placement="right" arrow>
                    <Box
                        component="a"
                        href="https://github.com/tingly-dev/tingly-box"
                        target="_blank"
                        rel="noopener noreferrer"
                        sx={{
                            width: 36,
                            height: 36,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            textDecoration: 'none',
                            cursor: 'pointer',
                            transition: 'transform 0.2s',
                            '&:hover': { transform: 'scale(1.08)' },
                        }}
                    >
                        <Box
                            component="img"
                            src="/assets/icon.svg"
                            alt="Tingly-Box"
                            sx={{ width: 36, height: 36, borderRadius: 8 }}
                        />
                    </Box>
                </Tooltip>
            </Box>

            {/* Activity Icons */}
            <Box sx={{ flex: 1, py: activityContainerPaddingY, overflowY: 'auto' }}>
                {activityItems.map((item) => {
                    const isActiveItem = activeActivity === item.key;
                    const shortLabel = item.label.length > 12 ? item.label.slice(0, 7) + '…' : item.label;

                    return (
                        <ListItemButton
                            key={item.key}
                            component={item.path && !item.children ? RouterLink : 'div'}
                            to={item.path && !item.children ? item.path : undefined}
                            onClick={() => onActivityClick(item)}
                            sx={activityItemSx({
                                '&:hover': {
                                    bgcolor: isActiveItem ? 'primary.dark' : 'action.hover',
                                    color: isActiveItem ? 'primary.contrastText' : 'primary.main',
                                },
                                ...(isActiveItem && {
                                    bgcolor: 'primary.main',
                                    color: 'primary.contrastText',
                                    '&::before': {
                                        content: '""',
                                        position: 'absolute',
                                        left: 0,
                                        top: '50%',
                                        transform: 'translateY(-50%)',
                                        width: 3,
                                        height: 28,
                                        bgcolor: 'primary.light',
                                        borderRadius: '0 2px 2px 0',
                                        boxShadow: '0 0 8px rgba(37, 99, 235, 0.5)',
                                    },
                                }),
                            })}
                        >
                            <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                                {item.icon}
                            </ListItemIcon>
                            <Typography
                                variant="caption"
                                sx={{
                                    fontSize: '0.65rem',
                                    fontWeight: isActiveItem ? 600 : 400,
                                    color: 'inherit',
                                    textAlign: 'center',
                                    lineHeight: 1.2,
                                    maxWidth: '100%',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                }}
                            >
                                {shortLabel}
                            </Typography>
                        </ListItemButton>
                    );
                })}

                <Divider sx={{ mx: 2, my: 1 }} />

                {/* Language menu - only show in normal mode */}
                {!zenEnabled && (
                    <Menu
                        anchorEl={languageMenuAnchorEl}
                        open={Boolean(languageMenuAnchorEl)}
                        onClose={handleLanguageMenuClose}
                        anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
                        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                        slotProps={{
                            paper: {
                                sx: {
                                    minWidth: 140,
                                    mt: 1,
                                },
                            },
                        }}
                    >
                        <MenuItem
                            selected={i18n.language === 'en'}
                            onClick={() => handleLanguageChange('en')}
                            sx={{ gap: 1.5 }}
                        >
                            <Typography>{t('system.language.en')}</Typography>
                        </MenuItem>
                        <MenuItem
                            selected={i18n.language === 'zh'}
                            onClick={() => handleLanguageChange('zh')}
                            sx={{ gap: 1.5 }}
                        >
                            <Typography>{t('system.language.zh')}</Typography>
                        </MenuItem>
                    </Menu>
                )}

                {/* Onboarding Quick Add Button - above Zen */}
                {!zenEnabled && (
                    <Tooltip title={t('layout.onboarding', { defaultValue: 'Quick Add Provider' })} placement="right" arrow>
                        <ListItemButton
                            component={RouterLink}
                            to="/onboarding"
                            sx={activityItemSx({
                                '&:hover': { bgcolor: 'action.hover', color: 'primary.main' },
                            })}
                        >
                            <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                                <IconWand size={22} />
                            </ListItemIcon>
                            <Typography variant="caption" sx={{ fontSize: '0.65rem', color: 'inherit', textAlign: 'center', lineHeight: 1.2 }}>
                                {t('layout.onboardingShort', { defaultValue: 'Onboard' })}
                            </Typography>
                        </ListItemButton>
                    </Tooltip>
                )}

                {/* Zen mode toggle button - only show in normal mode */}
                {!zenEnabled && (
                    <>
                        <Tooltip title={t('layout.activityBar.zenMode')} placement="right" arrow>
                            <ListItemButton
                                onClick={handleZenMenuClick}
                                sx={activityItemSx({
                                    '&:hover': { bgcolor: 'action.hover' },
                                })}
                            >
                                <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                                    <IconYinYang size={22} />
                                </ListItemIcon>
                                <Typography variant="caption" sx={{ fontSize: '0.65rem', color: 'inherit', textAlign: 'center', lineHeight: 1.2 }}>
                                    {t('common.zen')}
                                </Typography>
                            </ListItemButton>
                        </Tooltip>

                        {/* Zen Agent Selection Menu */}
                        <Menu
                            anchorEl={zenMenuAnchorEl}
                            open={Boolean(zenMenuAnchorEl)}
                            onClose={handleZenMenuClose}
                            anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
                            transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                            slotProps={{
                                paper: {
                                    sx: {
                                        minWidth: 180,
                                        mt: 1,
                                    },
                                },
                            }}
                        >
                            <MenuItem disabled sx={{ opacity: 0.6 }}>
                                <Typography variant="caption" sx={{ fontWeight: 600 }}>
                                    {t('layout.activityBar.enterZenMode')}
                                </Typography>
                            </MenuItem>
                            <MenuItem onClick={() => handleZenAgentSelect('/zen/claude_code')} sx={{ gap: 1.5 }}>
                                <Claude size={18} />
                                <Typography>Claude Code</Typography>
                            </MenuItem>
                            <MenuItem onClick={() => handleZenAgentSelect('/zen/codex')} sx={{ gap: 1.5 }}>
                                <Codex size={18} />
                                <Typography>Codex</Typography>
                            </MenuItem>
                            <MenuItem onClick={() => handleZenAgentSelect('/zen/opencode')} sx={{ gap: 1.5 }}>
                                <OpenCode size={18} />
                                <Typography>OpenCode</Typography>
                            </MenuItem>
                            {/*<MenuItem onClick={() => handleZenAgentSelect('/zen/xcode')} sx={{ gap: 1.5 }}>*/}
                            {/*    <Xcode size={18} />*/}
                            {/*    <Typography>Xcode</Typography>*/}
                            {/*</MenuItem>*/}
                            {/*<MenuItem onClick={() => handleZenAgentSelect('/zen/vscode')} sx={{ gap: 1.5 }}>*/}
                            {/*    <VSCode size={18} />*/}
                            {/*    <Typography>VS Code</Typography>*/}
                            {/*</MenuItem>*/}
                            {/*<MenuItem onClick={() => handleZenAgentSelect('/zen/openai')} sx={{ gap: 1.5 }}>*/}
                            {/*    <OpenAI size={18} />*/}
                            {/*    <Typography>OpenAI</Typography>*/}
                            {/*</MenuItem>*/}
                            {/*<MenuItem onClick={() => handleZenAgentSelect('/zen/anthropic')} sx={{ gap: 1.5 }}>*/}
                            {/*    <Anthropic size={18} />*/}
                            {/*    <Typography>Anthropic</Typography>*/}
                            {/*</MenuItem>*/}
                            {/*<MenuItem onClick={() => handleZenAgentSelect('/zen/agent')} sx={{ gap: 1.5 }}>*/}
                            {/*    <OpenClaw size={18} />*/}
                            {/*    <Typography>OpenClaw</Typography>*/}
                            {/*</MenuItem>*/}
                        </Menu>
                    </>
                )}

                {/* More button - only show in zen mode */}
                {zenEnabled && onMoreClick && (
                    <Tooltip title={t('layout.activityBar.more')} placement="right" arrow>
                        <ListItemButton
                            onClick={onMoreClick}
                            sx={activityItemSx({
                                '&:hover': { bgcolor: 'action.hover' },
                            })}
                        >
                            <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                                <IconDots size={22} />
                            </ListItemIcon>
                            <Typography variant="caption" sx={{ fontSize: '0.65rem', color: 'inherit', textAlign: 'center', lineHeight: 1.2 }}>
                                {t('layout.activityBar.more')}
                            </Typography>
                        </ListItemButton>
                    </Tooltip>
                )}

                {/* Exit Zen button - only show in zen mode */}
                {zenEnabled && onZenToggle && (
                    <Tooltip title={t('layout.activityBar.exitZen')} placement="right" arrow>
                        <ListItemButton
                            onClick={onZenToggle}
                            sx={activityItemSx({
                                '&:hover': { bgcolor: 'action.hover' },
                            })}
                        >
                            <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                                <IconYinYang size={22} />
                            </ListItemIcon>
                            <Typography variant="caption" sx={{ fontSize: '0.65rem', color: 'inherit', textAlign: 'center', lineHeight: 1.2 }}>
                                {t('layout.activityBar.exitZen')}
                            </Typography>
                        </ListItemButton>
                    </Tooltip>
                )}
            </Box>

            {/* Feedback button - bottom-left, above language icon */}
            <Box
                sx={{
                    py: 0.5,
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexShrink: 0,
                }}
            >
                <Tooltip title={t('layout.activityBar.feedbackTooltip')} placement="right" arrow>
                    <ListItemButton
                        component="a"
                        href="https://github.com/tingly-dev/tingly-box/issues/new/choose"
                        target="_blank"
                        rel="noopener noreferrer"
                        sx={{
                            minHeight: 48,
                            mx: 0.5,
                            px: activityItemPaddingX,
                            py: 0.75,
                            flexDirection: 'column',
                            alignItems: 'center',
                            justifyContent: 'center',
                            gap: 0.25,
                            position: 'relative',
                            color: 'text.secondary',
                            borderRadius: activityItemRadius,
                            cursor: 'pointer',
                            '&:hover': { bgcolor: 'action.hover', color: 'primary.main' },
                        }}
                    >
                        <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                            <IconMessageReport size={22} />
                        </ListItemIcon>
                        <Typography variant="caption" sx={{ fontSize: '0.65rem', color: 'inherit', textAlign: 'center', lineHeight: 1.2 }}>
                            {t('layout.activityBar.feedback')}
                        </Typography>
                    </ListItemButton>
                </Tooltip>
            </Box>

            {/* Language button - bottom-left, above user icon */}
            {!zenEnabled && (
                <Box
                    sx={{
                        py: 0.5,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        flexShrink: 0,
                    }}
                >
                    <Tooltip title={t('system.language.title')} placement="right" arrow>
                        <ListItemButton
                            onClick={handleLanguageMenuClick}
                            sx={{
                                minHeight: 48,
                                mx: 0.5,
                                px: activityItemPaddingX,
                                py: 0.75,
                                flexDirection: 'column',
                                alignItems: 'center',
                                justifyContent: 'center',
                                gap: 0.25,
                                position: 'relative',
                                color: 'text.secondary',
                                borderRadius: activityItemRadius,
                                cursor: 'pointer',
                                '&:hover': { bgcolor: 'action.hover', color: 'primary.main' },
                            }}
                        >
                            <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                                <IconLanguage size={22} />
                            </ListItemIcon>
                            <Typography variant="caption" sx={{ fontSize: '0.5rem', color: 'inherit', textAlign: 'center', lineHeight: 1.1 }}>
                                {i18n.language === 'zh' ? '中文' : 'EN'}
                            </Typography>
                        </ListItemButton>
                    </Tooltip>
                </Box>
            )}

            {/* Bottom: User icon */}
            <Box
                sx={{
                    py: 0.5,
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexShrink: 0,
                    gap: 0.5,
                    height: footerHeight,
                }}
            >
                {/* User icon */}
                <Tooltip title={t('layout.activityBar.click')} placement="right" arrow>
                    <ListItemButton
                        onClick={onUserClick}
                        sx={{
                            minHeight: 48,
                            mx: 0.5,
                            px: activityItemPaddingX,
                            py: 0.75,
                            flexDirection: 'column',
                            alignItems: 'center',
                            justifyContent: 'center',
                            gap: 0.25,
                            position: 'relative',
                            color: 'text.secondary',
                            borderRadius: activityItemRadius,
                            cursor: 'pointer',
                            '&:hover': { bgcolor: 'action.hover', color: 'text.primary' },
                        }}
                    >
                        <ListItemIcon sx={{ minWidth: 0, color: 'inherit', justifyContent: 'center' }}>
                            <IconUser size={20} />
                        </ListItemIcon>
                    </ListItemButton>
                </Tooltip>
            </Box>
        </Box>
    );
};
