import { Box, Divider, Menu, MenuItem, Typography } from '@mui/material';
import { IconDots, IconUser, IconSettings, IconBrush, IconSparkles } from '@tabler/icons-react';
import React, { ReactNode, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useActivityItems } from './useActivityItems';
import { ZenButton } from './ZenButton';
import { headerHeight } from './constants';
import { Claude, Codex, OpenCode, Xcode, VSCode, OpenAI, Anthropic, OpenClaw } from '../components/BrandIcons';

/**
 * Navigation action types for zen mode
 */
export type ZenNavAction = 'path' | 'profile' | 'more' | 'theme' | 'system' | 'zen';

/**
 * Zen navigation item configuration
 */
export interface ZenNavItem {
  /** Button icon */
  icon: ReactNode;
  /** Button label */
  label: string;
  /** Action type */
  action: ZenNavAction;
  /** Navigation path if action is 'path' */
  path?: string;
}

/**
 * Zen layout configuration for an agent
 */
export interface ZenLayoutConfig {
  /** Agent identifier */
  agent: string;
  /** Primary navigation button (agent page) */
  primaryButton: ZenNavItem;
  /** Whether to show Profile button */
  showProfile: boolean;
  /** Additional buttons to include */
  additionalButtons: ZenNavItem[];
}

/**
 * Props for ZenNavigationBar
 */
export interface ZenNavigationBarProps {
  /** Current zen agent */
  agent: string;
  /** Called when zen mode should be exited */
  onExitZen: () => void;
}

/**
 * Zen navigation bar component
 *
 * Displays a horizontal navigation bar with agent-specific buttons.
 * Replaces the ActivityBar + Sidebar layout when zen mode is enabled.
 *
 * @example
 * ```tsx
 * <ZenNavigationBar
 *   agent="claude_code"
 *   onExitZen={() => setZenMode('')}
 * />
 * ```
 */
export const ZenNavigationBar: React.FC<ZenNavigationBarProps> = ({ agent, onExitZen }) => {
  const navigate = useNavigate();
  const activityItems = useActivityItems();
  const [moreMenuAnchor, setMoreMenuAnchor] = useState<HTMLElement | null>(null);

  // Get zen layout configuration for current agent
  const config = getZenLayoutConfig(agent);

  const handleButtonClick = (item: ZenNavItem) => {
    switch (item.action) {
      case 'path':
        if (item.path) {
          navigate(item.path);
        }
        break;
      case 'more':
        setMoreMenuAnchor(document.getElementById('zen-more-button'));
        break;
      case 'system':
        navigate('/system');
        break;
      case 'zen':
        onExitZen();
        break;
      case 'profile':
        // Profile menu is handled by ZenProfileSelector (parent component)
        break;
      case 'theme':
        // Theme menu is handled by parent component
        break;
    }
  };

  const handleMoreMenuClose = () => {
    setMoreMenuAnchor(null);
  };

  const handleMoreMenuItemClick = (path: string) => {
    navigate(path);
    handleMoreMenuClose();
  };

  return (
    <Box
      sx={{
        height: headerHeight,
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        px: 2,
        borderBottom: '1px solid',
        borderColor: 'divider',
        bgcolor: 'background.paper',
      }}
    >
      {/* Primary button (agent) */}
      <ZenButton
        icon={config.primaryButton.icon}
        label={config.primaryButton.label}
        onClick={() => handleButtonClick(config.primaryButton)}
        active
      />

      {/* Profile button (if applicable) */}
      {config.showProfile && (
        <ZenButton
          icon={<IconUser size={22} />}
          label="Profile"
          onClick={() => handleButtonClick({ action: 'profile', icon: null, label: '' })}
        />
      )}

      {/* Additional buttons */}
      {config.additionalButtons.map((button) => (
        <ZenButton
          key={button.label}
          icon={button.icon}
          label={button.label}
          onClick={() => handleButtonClick(button)}
          {...(button.action === 'more' && { id: 'zen-more-button' })}
        />
      ))}

      {/* More Menu */}
      <ZenMoreMenu
        open={Boolean(moreMenuAnchor)}
        anchorEl={moreMenuAnchor}
        onClose={handleMoreMenuClose}
        onItemClick={handleMoreMenuItemClick}
        activityItems={activityItems}
        currentAgent={agent}
      />
    </Box>
  );
};

/**
 * Zen more menu component
 */
interface ZenMoreMenuProps {
  open: boolean;
  anchorEl: HTMLElement | null;
  onClose: () => void;
  onItemClick: (path: string) => void;
  activityItems: ReturnType<typeof useActivityItems>;
  currentAgent: string;
}

const ZenMoreMenu: React.FC<ZenMoreMenuProps> = ({
  open,
  anchorEl,
  onClose,
  onItemClick,
  activityItems,
  currentAgent,
}) => {
  return (
    <Menu
      open={open}
      anchorEl={anchorEl}
      onClose={onClose}
      anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      transformOrigin={{ vertical: 'top', horizontal: 'left' }}
      slotProps={{
        paper: {
          sx: { minWidth: 200, maxHeight: 400 },
        },
      }}
    >
      {activityItems.map((activity) => {
        // Skip current agent's activity
        if (activity.key === getActivityKeyForAgent(currentAgent)) {
          return null;
        }

        return (
          <div key={activity.key}>
            <MenuItem
              sx={{ opacity: 0.7, pointerEvents: 'none' }}
              disabled
            >
              <Typography variant="caption" sx={{ fontWeight: 600 }}>
                {activity.label}
              </Typography>
            </MenuItem>
            {activity.children?.map((child) => {
              if (child.type === 'divider') {
                return <Divider key={`divider-${Math.random()}`} />;
              }
              return (
                <MenuItem
                  key={child.path}
                  onClick={() => child.path && onItemClick(child.path)}
                >
                  {child.icon}
                  <Typography sx={{ ml: 1 }}>{child.label}</Typography>
                </MenuItem>
              );
            })}
            <Divider />
          </div>
        );
      })}
    </Menu>
  );
};

/**
 * Get activity key for an agent
 */
function getActivityKeyForAgent(agent: string): string {
  const keyMap: Record<string, string> = {
    claude_code: 'scenario',
    codex: 'scenario',
    opencode: 'scenario',
    xcode: 'scenario',
    vscode: 'scenario',
    openai: 'scenario',
    anthropic: 'scenario',
    agent: 'scenario',
  };
  return keyMap[agent] || 'scenario';
}

/**
 * Get zen layout configuration for an agent
 */
function getZenLayoutConfig(agent: string): ZenLayoutConfig {
  // Use IconSparkles as the zen icon
  const IconZen = IconSparkles;

  const configs: Record<string, ZenLayoutConfig> = {
    claude_code: {
      agent: 'claude_code',
      primaryButton: {
        icon: <Claude size={22} />,
        label: 'Claude',
        action: 'path',
        path: '/use-claude-code',
      },
      showProfile: true,
      additionalButtons: [
        { icon: <IconDots size={22} />, label: 'More', action: 'more' },
        { icon: <IconSettings size={22} />, label: 'System', action: 'system' },
        { icon: <IconBrush size={22} />, label: 'Theme', action: 'theme' },
        { icon: <IconZen size={22} />, label: 'Zen', action: 'zen' },
      ],
    },
    codex: {
      agent: 'codex',
      primaryButton: {
        icon: <Codex size={22} />,
        label: 'Codex',
        action: 'path',
        path: '/use-codex',
      },
      showProfile: true,
      additionalButtons: [
        { icon: <IconDots size={22} />, label: 'More', action: 'more' },
        { icon: <IconSettings size={22} />, label: 'System', action: 'system' },
        { icon: <IconBrush size={22} />, label: 'Theme', action: 'theme' },
        { icon: <IconZen size={22} />, label: 'Zen', action: 'zen' },
      ],
    },
    opencode: {
      agent: 'opencode',
      primaryButton: {
        icon: <OpenCode size={22} />,
        label: 'OpenCode',
        action: 'path',
        path: '/use-opencode',
      },
      showProfile: true,
      additionalButtons: [
        { icon: <IconDots size={22} />, label: 'More', action: 'more' },
        { icon: <IconSettings size={22} />, label: 'System', action: 'system' },
        { icon: <IconBrush size={22} />, label: 'Theme', action: 'theme' },
        { icon: <IconZen size={22} />, label: 'Zen', action: 'zen' },
      ],
    },
    xcode: {
      agent: 'xcode',
      primaryButton: {
        icon: <Xcode size={22} />,
        label: 'Xcode',
        action: 'path',
        path: '/use-xcode',
      },
      showProfile: true,
      additionalButtons: [
        { icon: <IconDots size={22} />, label: 'More', action: 'more' },
        { icon: <IconSettings size={22} />, label: 'System', action: 'system' },
        { icon: <IconBrush size={22} />, label: 'Theme', action: 'theme' },
        { icon: <IconZen size={22} />, label: 'Zen', action: 'zen' },
      ],
    },
    vscode: {
      agent: 'vscode',
      primaryButton: {
        icon: <VSCode size={22} />,
        label: 'VS Code',
        action: 'path',
        path: '/use-vscode',
      },
      showProfile: true,
      additionalButtons: [
        { icon: <IconDots size={22} />, label: 'More', action: 'more' },
        { icon: <IconSettings size={22} />, label: 'System', action: 'system' },
        { icon: <IconBrush size={22} />, label: 'Theme', action: 'theme' },
        { icon: <IconZen size={22} />, label: 'Zen', action: 'zen' },
      ],
    },
    openai: {
      agent: 'openai',
      primaryButton: {
        icon: <OpenAI size={22} />,
        label: 'OpenAI',
        action: 'path',
        path: '/use-openai',
      },
      showProfile: false,
      additionalButtons: [
        { icon: <IconDots size={22} />, label: 'More', action: 'more' },
        { icon: <IconSettings size={22} />, label: 'System', action: 'system' },
        { icon: <IconBrush size={22} />, label: 'Theme', action: 'theme' },
        { icon: <IconZen size={22} />, label: 'Zen', action: 'zen' },
      ],
    },
    anthropic: {
      agent: 'anthropic',
      primaryButton: {
        icon: <Anthropic size={22} />,
        label: 'Anthropic',
        action: 'path',
        path: '/use-anthropic',
      },
      showProfile: false,
      additionalButtons: [
        { icon: <IconDots size={22} />, label: 'More', action: 'more' },
        { icon: <IconSettings size={22} />, label: 'System', action: 'system' },
        { icon: <IconBrush size={22} />, label: 'Theme', action: 'theme' },
        { icon: <IconZen size={22} />, label: 'Zen', action: 'zen' },
      ],
    },
    agent: {
      agent: 'agent',
      primaryButton: {
        icon: <OpenClaw size={22} />,
        label: 'OpenClaw',
        action: 'path',
        path: '/use-agent',
      },
      showProfile: false,
      additionalButtons: [
        { icon: <IconDots size={22} />, label: 'More', action: 'more' },
        { icon: <IconSettings size={22} />, label: 'System', action: 'system' },
        { icon: <IconBrush size={22} />, label: 'Theme', action: 'theme' },
        { icon: <IconZen size={22} />, label: 'Zen', action: 'zen' },
      ],
    },
  };

  return configs[agent] || configs.claude_code;
}

export default ZenNavigationBar;
