import { useState, useCallback, useRef, useEffect } from 'react';
import {
  Box,
  TextField,
  InputAdornment,
  IconButton,
  Tooltip,
  Paper,
  Popper,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  ClickAwayListener,
  Typography,
} from '@mui/material';
import {
  Search as SearchIcon,
  Clear,
  History,
} from '@mui/icons-material';

interface MemorySearchBarProps {
  value: string;
  onChange: (value: string) => void;
  onSearch: (value: string) => void;
  isLoading?: boolean;
  placeholder?: string;
  recentSearches?: string[];
  onClearRecent?: () => void;
}

const MemorySearchBar: React.FC<MemorySearchBarProps> = ({
  value,
  onChange,
  onSearch,
  isLoading = false,
  placeholder = 'Search memories...',
  recentSearches = [],
  onClearRecent,
}) => {
  const [showSuggestions, setShowSuggestions] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const anchorRef = useRef<HTMLDivElement>(null);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && value.trim()) {
      onSearch(value.trim());
      setShowSuggestions(false);
    } else if (e.key === 'Escape') {
      setShowSuggestions(false);
      inputRef.current?.blur();
    }
  }, [value, onSearch]);

  const handleClear = useCallback(() => {
    onChange('');
    onSearch('');
    inputRef.current?.focus();
  }, [onChange, onSearch]);

  const handleFocus = useCallback(() => {
    if (recentSearches.length > 0 && !value) {
      setShowSuggestions(true);
    }
  }, [recentSearches, value]);

  const handleSuggestionClick = useCallback((suggestion: string) => {
    onChange(suggestion);
    onSearch(suggestion);
    setShowSuggestions(false);
  }, [onChange, onSearch]);

  // Keyboard shortcut: / to focus search
  useEffect(() => {
    const handleGlobalKeyDown = (e: KeyboardEvent) => {
      if (e.key === '/' && document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
        e.preventDefault();
        inputRef.current?.focus();
      }
    };

    window.addEventListener('keydown', handleGlobalKeyDown);
    return () => window.removeEventListener('keydown', handleGlobalKeyDown);
  }, []);

  const showRecent = showSuggestions && recentSearches.length > 0 && !value;

  return (
    <ClickAwayListener onClickAway={() => setShowSuggestions(false)}>
      <Box ref={anchorRef} sx={{ position: 'relative' }}>
        <TextField
          inputRef={inputRef}
          fullWidth
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={handleKeyDown}
          onFocus={handleFocus}
          placeholder={placeholder}
          slotProps={{
            input: {
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon
                    sx={{
                      color: value ? 'primary.main' : 'text.secondary',
                      transition: 'color 0.2s',
                    }}
                  />
                </InputAdornment>
              ),
              endAdornment: value && (
                <InputAdornment position="end">
                  <IconButton
                    size="small"
                    onClick={handleClear}
                    disabled={isLoading}
                  >
                    <Clear fontSize="small" />
                  </IconButton>
                </InputAdornment>
              ),
              sx: {
                fontSize: '1rem',
                '&::placeholder': {
                  opacity: 0.7,
                },
              },
            },
          }}
          sx={{
            '& .MuiOutlinedInput-root': {
              borderRadius: 2,
              bgcolor: 'background.paper',
              transition: 'box-shadow 0.2s, border-color 0.2s',
              '&:hover': {
                boxShadow: 1,
              },
              '&.Mui-focused': {
                boxShadow: 2,
              },
            },
          }}
        />

        {/* Recent Searches Popper */}
        <Popper
          open={showRecent}
          anchorEl={anchorRef.current}
          placement="bottom-start"
          style={{ zIndex: 1300 }}
        >
          <Paper
            sx={{
              mt: 0.5,
              minWidth: anchorRef.current?.offsetWidth || 300,
              maxHeight: 240,
              overflow: 'auto',
              boxShadow: 3,
            }}
          >
            <Box sx={{ p: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderBottom: 1, borderColor: 'divider' }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <History fontSize="small" color="action" />
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
                  Recent Searches
                </Typography>
              </Box>
              {onClearRecent && (
                <Tooltip title="Clear history">
                  <IconButton size="small" onClick={onClearRecent}>
                    <Clear fontSize="small" />
                  </IconButton>
                </Tooltip>
              )}
            </Box>
            <List dense disablePadding>
              {recentSearches.map((search, index) => (
                <ListItem key={index} disablePadding>
                  <ListItemButton
                    onClick={() => handleSuggestionClick(search)}
                    sx={{ py: 0.75 }}
                  >
                    <ListItemText
                      primary={search}
                      primaryTypographyProps={{ fontSize: '0.875rem' }}
                    />
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Paper>
        </Popper>

        {/* Keyboard hint */}
        <Box
          sx={{
            position: 'absolute',
            right: value ? 48 : 12,
            top: '50%',
            transform: 'translateY(-50%)',
            pointerEvents: 'none',
          }}
        >
          {!value && (
            <Typography
              variant="caption"
              sx={{
                px: 0.75,
                py: 0.25,
                bgcolor: 'grey.100',
                borderRadius: 0.5,
                color: 'text.secondary',
                fontSize: '0.7rem',
                fontFamily: 'monospace',
              }}
            >
              /
            </Typography>
          )}
        </Box>
      </Box>
    </ClickAwayListener>
  );
};

export default MemorySearchBar;
