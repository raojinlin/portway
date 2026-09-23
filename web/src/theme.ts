import { theme, type ThemeConfig } from 'antd'

export type ThemeMode = 'light' | 'dark'

export interface Palette {
  bg: string
  board: string
  boardRaised: string
  rule: string
  ink: string
  inkDim: string
  inkFaint: string
  good: string
  pending: string
  bad: string
  off: string
  accent: string
}

const darkPalette: Palette = {
  bg: '#181A1D',
  board: '#202328',
  boardRaised: '#22252A',
  rule: '#363A40',
  ink: '#E4E7EB',
  inkDim: '#ABB2BC',
  inkFaint: '#929BA7',
  good: '#69B68A',
  pending: '#D5A457',
  bad: '#EB817B',
  off: '#929BA7',
  accent: '#82A9E2',
}

const lightPalette: Palette = {
  bg: '#F3F5F7',
  board: '#FFFFFF',
  boardRaised: '#F5F6F8',
  rule: '#DFE2E6',
  ink: '#24292F',
  inkDim: '#59636F',
  inkFaint: '#687380',
  good: '#287B4B',
  pending: '#946400',
  bad: '#BF3D35',
  off: '#687380',
  accent: '#315F9B',
}

export function getPalette(mode: ThemeMode): Palette {
  return mode === 'dark' ? darkPalette : lightPalette
}

export const fontSans = `'IBM Plex Sans', ui-sans-serif, system-ui, -apple-system, sans-serif`
export const fontCond = `'IBM Plex Sans Condensed', 'IBM Plex Sans', ui-sans-serif, system-ui, sans-serif`
export const fontMono = `'IBM Plex Mono', ui-monospace, SFMono-Regular, Menlo, monospace`

export function buildAntdTheme(mode: ThemeMode): ThemeConfig {
  const p = getPalette(mode)
  return {
    algorithm: mode === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
    token: {
      colorBgBase: p.bg,
      colorBgContainer: p.board,
      colorBgElevated: p.boardRaised,
      colorBorder: p.rule,
      colorBorderSecondary: p.rule,
      colorPrimary: p.accent,
      colorTextBase: p.ink,
      colorTextSecondary: p.inkDim,
      colorError: p.bad,
      colorWarning: p.pending,
      fontFamily: fontSans,
      borderRadius: 3,
      controlHeight: 34,
      boxShadow: 'none',
      boxShadowSecondary: 'none',
    },
    components: {
      Button: { fontWeight: 400, primaryShadow: 'none', defaultShadow: 'none', dangerShadow: 'none' },
      Modal: { borderRadiusLG: 4 },
      Input: { activeShadow: 'none' },
      Segmented: {
        trackBg: p.boardRaised,
        itemColor: p.inkDim,
        itemHoverBg: mode === 'dark' ? '#30353D' : '#E9ECF0',
        itemHoverColor: p.ink,
        itemActiveBg: mode === 'dark' ? '#383F49' : '#DFE4EB',
        itemSelectedBg: mode === 'dark' ? '#315680' : '#E4EDF9',
        itemSelectedColor: mode === 'dark' ? '#F0F6FF' : p.accent,
      },
    },
  }
}
