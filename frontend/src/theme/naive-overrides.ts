import type { GlobalThemeOverrides } from 'naive-ui'

/**
 * Naive UI theme overrides for Papeer.
 *
 * Colors based on our design token palette:
 * - Primary: Violet (#7c5cff)
 * - Info: Violet (#7c5cff)
 * - Success: Green (#22c55e)
 * - Warning: Amber (#f59e0b)
 * - Error: Red (#ef4444)
 */
export const themeOverrides: GlobalThemeOverrides = {
  common: {
    // Primary — Violet
    primaryColor: '#7c5cff',
    primaryColorHover: '#9880ff',
    primaryColorPressed: '#6a44f0',
    primaryColorSuppl: '#7dd3fc',

    // Info — same as primary
    infoColor: '#7c5cff',
    infoColorHover: '#9880ff',
    infoColorPressed: '#6a44f0',
    infoColorSuppl: '#7dd3fc',

    // Success — Green
    successColor: '#22c55e',
    successColorHover: '#4ade80',
    successColorPressed: '#16a34a',
    successColorSuppl: '#86efac',

    // Warning — Amber
    warningColor: '#f59e0b',
    warningColorHover: '#fbbf24',
    warningColorPressed: '#d97706',
    warningColorSuppl: '#fcd34d',

    // Error — Red
    errorColor: '#ef4444',
    errorColorHover: '#f87171',
    errorColorPressed: '#dc2626',
    errorColorSuppl: '#fca5a5',

    // Typography
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
    fontFamilyMono: "'JetBrains Mono', 'SF Mono', 'Fira Code', monospace",
    fontSize: '15px',
    fontSizeMini: '11px',
    fontSizeTiny: '11px',
    fontSizeSmall: '13px',
    fontSizeMedium: '15px',
    fontSizeLarge: '16px',
    fontSizeHuge: '18px',

    lineHeight: '1.6',

    // Border radius
    borderRadius: '8px',
    borderRadiusSmall: '4px',

    // Backgrounds (dark mode — slate palette)
    bodyColor: '#0f172a',
    cardColor: '#1e293b',
    modalColor: '#1e293b',
    popoverColor: '#253347',
    tableColor: '#1e293b',
    inputColor: '#0f172a',

    // Text
    textColorBase: '#f8fafc',
    textColor1: '#f1f5f9',
    textColor2: '#cbd5e1',
    textColor3: '#94a3b8',

    // Borders
    borderColor: 'rgba(148, 163, 184, 0.12)',
    dividerColor: 'rgba(148, 163, 184, 0.08)',

    // Hover/active
    hoverColor: 'rgba(124, 92, 255, 0.08)',
    pressedColor: 'rgba(124, 92, 255, 0.12)',
  },

  Card: {
    color: '#1e293b',
    borderColor: 'rgba(148, 163, 184, 0.10)',
    borderRadius: '12px',
    paddingMedium: '20px',
    paddingSmall: '16px',
    titleFontSizeMedium: '18px',
    titleFontSizeSmall: '15px',
    titleFontWeight: '600',
    boxShadow: '0 1px 3px rgba(0, 0, 0, 0.2), 0 1px 2px rgba(0, 0, 0, 0.12)',
  },

  Button: {
    borderRadiusMedium: '8px',
    borderRadiusSmall: '6px',
    borderRadiusTiny: '4px',
    fontWeight: '500',
    fontSizeMedium: '14px',
    fontSizeSmall: '13px',
    fontSizeTiny: '12px',
    heightMedium: '36px',
    heightSmall: '32px',
    heightTiny: '28px',
    paddingMedium: '0 16px',
    paddingSmall: '0 12px',
    // Primary button colors
    colorPrimary: '#7c5cff',
    colorHoverPrimary: '#9880ff',
    colorPressedPrimary: '#6a44f0',
    textColorPrimary: '#ffffff',
    textColorHoverPrimary: '#ffffff',
    textColorPressedPrimary: '#ffffff',
    // Ghost/text button colors
    textColorGhostPrimary: '#7c5cff',
    textColorGhostHoverPrimary: '#9880ff',
    textColorGhostPressedPrimary: '#6a44f0',
    // Secondary (default) button
    color: 'rgba(148, 163, 184, 0.08)',
    colorHover: 'rgba(148, 163, 184, 0.12)',
    colorPressed: 'rgba(148, 163, 184, 0.16)',
    border: '1px solid rgba(148, 163, 184, 0.2)',
    borderHover: '1px solid rgba(124, 92, 255, 0.4)',
    borderPressed: '1px solid rgba(124, 92, 255, 0.5)',
    textColor: '#cbd5e1',
    textColorHover: '#e2e8f0',
    textColorPressed: '#f1f5f9',
  },

  Menu: {
    borderRadius: '8px',
    itemHeight: '40px',
    fontSize: '14px',
    itemTextColor: '#94a3b8',
    itemTextColorHover: '#e2e8f0',
    itemTextColorActive: '#7c5cff',
    itemTextColorActiveHover: '#9880ff',
    itemColorActive: 'rgba(124, 92, 255, 0.1)',
    itemColorActiveHover: 'rgba(124, 92, 255, 0.15)',
    itemIconColor: '#94a3b8',
    itemIconColorHover: '#e2e8f0',
    itemIconColorActive: '#7c5cff',
    itemIconColorActiveHover: '#9880ff',
  },

  Tabs: {
    tabFontSizeMedium: '14px',
    tabFontSizeSmall: '13px',
    tabFontWeight: '500',
    tabFontWeightActive: '600',
    tabTextColorLine: '#94a3b8',
    tabTextColorActiveLine: '#7c5cff',
    tabTextColorHoverLine: '#cbd5e1',
    barColor: '#7c5cff',
    tabTextColorSegment: '#94a3b8',
    tabTextColorActiveSegment: '#f8fafc',
    colorSegment: 'rgba(148, 163, 184, 0.06)',
    tabColorSegment: 'transparent',
  },

  Tag: {
    borderRadius: '6px',
    fontSizeSmall: '12px',
    fontSizeMedium: '13px',
    fontSizeTiny: '11px',
    heightSmall: '24px',
    heightMedium: '28px',
    heightTiny: '20px',
    // Softer tint backgrounds for tag types
    colorInfo: 'rgba(124, 92, 255, 0.12)',
    borderInfo: '1px solid rgba(124, 92, 255, 0.2)',
    textColorInfo: '#9880ff',
    colorSuccess: 'rgba(34, 197, 94, 0.12)',
    borderSuccess: '1px solid rgba(34, 197, 94, 0.2)',
    textColorSuccess: '#4ade80',
    colorWarning: 'rgba(245, 158, 11, 0.12)',
    borderWarning: '1px solid rgba(245, 158, 11, 0.2)',
    textColorWarning: '#fbbf24',
    colorError: 'rgba(239, 68, 68, 0.12)',
    borderError: '1px solid rgba(239, 68, 68, 0.2)',
    textColorError: '#f87171',
  },

  Input: {
    color: 'rgba(15, 23, 42, 0.6)',
    colorFocus: 'rgba(15, 23, 42, 0.8)',
    border: '1px solid rgba(148, 163, 184, 0.15)',
    borderHover: '1px solid rgba(124, 92, 255, 0.4)',
    borderFocus: '1px solid #7c5cff',
    borderRadius: '8px',
    caretColor: '#7c5cff',
    fontSizeMedium: '14px',
    fontSizeSmall: '13px',
    heightMedium: '36px',
    heightSmall: '32px',
  },

  DataTable: {
    borderRadius: '8px',
    fontSizeSmall: '13px',
    fontSizeMedium: '14px',
    thColor: 'rgba(30, 41, 59, 0.8)',
    thTextColor: '#94a3b8',
    thFontWeight: '600',
    tdColor: 'transparent',
    tdColorHover: 'rgba(124, 92, 255, 0.04)',
    tdTextColor: '#e2e8f0',
    borderColor: 'rgba(148, 163, 184, 0.06)',
  },

  Statistic: {
    valueFontSize: '28px',
    labelFontSize: '13px',
    labelTextColor: '#94a3b8',
  },

  Collapse: {
    titleFontSize: '15px',
    titleFontWeight: '600',
    titleTextColor: '#e2e8f0',
    arrowColor: '#64748b',
    dividerColor: 'rgba(148, 163, 184, 0.08)',
  },

  Popover: {
    color: '#253347',
    borderRadius: '10px',
    fontSize: '14px',
    padding: '12px',
  },

  Modal: {
    color: '#1e293b',
    borderRadius: '12px',
    titleFontSize: '18px',
    fontSize: '15px',
    boxShadow: '0 8px 32px rgba(0, 0, 0, 0.5)',
  },

  Dialog: {
    borderRadius: '12px',
    titleFontSize: '16px',
    fontSize: '14px',
    padding: '20px 24px',
    iconMargin: '0 8px 0 0',
  },

  Pagination: {
    itemBorderRadius: '6px',
    itemFontSize: '13px',
    itemColorActive: 'rgba(124, 92, 255, 0.15)',
    itemTextColorActive: '#7c5cff',
    itemBorderActive: '1px solid rgba(124, 92, 255, 0.3)',
  },

  Progress: {
    fillColor: '#7c5cff',
    railColor: 'rgba(148, 163, 184, 0.1)',
    fontSizeLine: '13px',
  },

  Alert: {
    borderRadius: '10px',
    fontSize: '14px',
  },

  Badge: {
    fontSize: '11px',
  },

  Divider: {
    color: 'rgba(148, 163, 184, 0.08)',
  },

  Scrollbar: {
    color: 'rgba(148, 163, 184, 0.2)',
    colorHover: 'rgba(148, 163, 184, 0.35)',
  },

  Switch: {
    railColorActive: '#7c5cff',
  },

  Rate: {
    colorFilled: '#f59e0b',
  },

  Dropdown: {
    borderRadius: '10px',
    optionFontSize: '14px',
    padding: '6px',
    optionColorHover: 'rgba(124, 92, 255, 0.08)',
  },

  Tooltip: {
    borderRadius: '8px',
    fontSize: '13px',
    color: '#334155',
    textColor: '#e2e8f0',
  },
}
