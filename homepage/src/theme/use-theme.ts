import { useTheme as useMuiTheme } from "@mui/material/styles";
import { AppTheme, UseThemeResult } from "./types";
import { colors } from "./index";

/**
 * Custom hook to access the MUI theme with additional custom properties
 * @returns {UseThemeResult} The theme object and extracted colors
 */
export const useTheme = (): UseThemeResult => {
  const theme = useMuiTheme() as AppTheme;

  return {
    theme,
    colors,
  };
};

export default useTheme;