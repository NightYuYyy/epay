// Naive UI theme overrides aligned to the Stripe-inspired design system.
// Re-export `lightTheme` and `darkTheme` consumers via NConfigProvider.
import type { GlobalThemeOverrides } from 'naive-ui'

const ink = '#0d253d'
const inkSecondary = '#273951'
const inkMute = '#64748d'
const inkMute2 = '#61718a'
const primary = '#533afd'
const primaryDeep = '#4434d4'
const primaryPress = '#2e2b8c'
const primarySoft = '#665efd'
const canvas = '#ffffff'
const canvasSoft = '#f6f9fc'
const hairline = '#e3e8ee'
const error = '#d92d20'
const success = '#1ab87a'
const warning = '#c4880f'

export const stripeLightOverrides: GlobalThemeOverrides = {
  common: {
    fontFamily: "'Inter', 'SF Pro Text', system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif",
    fontFamilyMono: "'JetBrains Mono', 'SF Mono', ui-monospace, Consolas, monospace",
    primaryColor: primary,
    primaryColorHover: primarySoft,
    primaryColorPressed: primaryPress,
    primaryColorSuppl: primaryDeep,
    infoColor: primary,
    successColor: success,
    warningColor: warning,
    errorColor: error,
    textColorBase: ink,
    textColor1: ink,
    textColor2: inkSecondary,
    textColor3: inkMute,
    textColorDisabled: inkMute2,
    placeholderColor: inkMute,
    iconColor: inkMute,
    bodyColor: canvas,
    cardColor: canvas,
    modalColor: canvas,
    popoverColor: canvas,
    tableColor: canvas,
    tableColorHover: canvasSoft,
    tableColorStriped: canvasSoft,
    inputColor: canvas,
    inputColorDisabled: canvasSoft,
    borderColor: hairline,
    dividerColor: hairline,
    closeIconColor: inkMute,
    borderRadius: '8px',
    borderRadiusSmall: '6px',
    fontSize: '14px',
    fontSizeSmall: '13px',
    fontWeightStrong: '600',
    boxShadow1: '0 1px 3px -1px rgba(13, 37, 61, 0.10), 0 1px 2px -1px rgba(13, 37, 61, 0.06)',
    boxShadow2: '0 2px 6px -1px rgba(50, 50, 93, 0.10), 0 1px 3px -1px rgba(0, 0, 0, 0.05)',
    boxShadow3: '0 10px 30px -8px rgba(50, 50, 93, 0.18), 0 4px 12px -4px rgba(13, 37, 61, 0.08)',
  },
  Button: {
    fontWeight: '500',
    borderRadiusMedium: '8px',
    paddingMedium: '0 18px',
    heightMedium: '36px',
  },
  Card: {
    borderRadius: '12px',
    paddingMedium: '20px 24px',
    titleFontWeight: '500',
    titleTextColor: ink,
  },
  Menu: {
    itemColorActive: 'rgba(83, 58, 253, 0.12)',
    itemColorActiveHover: 'rgba(83, 58, 253, 0.18)',
    itemTextColorActive: primary,
    itemTextColorActiveHover: primary,
    itemTextColorHover: primary,
    itemIconColorActive: primary,
    arrowColorActive: primary,
    borderRadius: '8px',
    fontSize: '14px',
  },
  DataTable: {
    thColor: canvasSoft,
    thTextColor: inkMute,
    thFontWeight: '600',
    tdColorHover: canvasSoft,
    borderColor: hairline,
    borderRadius: '10px',
  },
  Tag: {
    borderRadius: '6px',
    fontWeightStrong: '500',
  },
  Input: {
    borderRadius: '8px',
    border: `1px solid ${hairline}`,
    borderHover: `1px solid ${primarySoft}`,
    borderFocus: `1px solid ${primary}`,
    boxShadowFocus: '0 0 0 3px rgba(83, 58, 253, 0.20)',
    color: canvas,
  },
}
