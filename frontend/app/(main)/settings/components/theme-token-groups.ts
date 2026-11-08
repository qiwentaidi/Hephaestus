export type TokenGroupId = 'surfaces' | 'sidebar' | 'tones'

const tone = (name: string) => [
  `--tone-${name}`,
  `--tone-${name}-soft`,
  `--tone-${name}-border`,
  `--tone-${name}-icon-bg`,
  `--tone-${name}-icon-border`,
]

export const PRIMARY_TOKENS = [
  '--primary',
  '--primary-foreground',
  '--ring',
]

export const SUCCESS_TOKENS = ['--success', '--success-foreground', ...tone('green')]

export const WARNING_TOKENS = ['--warning', '--warning-foreground', ...tone('amber')]

export const INFO_TOKENS = ['--info', '--info-foreground', ...tone('blue')]

export const DESTRUCTIVE_TOKENS = [
  '--destructive',
  '--destructive-foreground',
  '--error',
  '--error-foreground',
  ...tone('red'),
]

export const SIDEBAR_TOKENS = [
  '--sidebar',
  '--sidebar-foreground',
  '--sidebar-primary',
  '--sidebar-primary-foreground',
  '--sidebar-accent',
  '--sidebar-accent-foreground',
  '--sidebar-border',
  '--sidebar-ring',
  '--sidebar-hover',
]

export const SURFACE_TOKENS = [
  '--background',
  '--foreground',
  '--card',
  '--card-foreground',
  '--popover',
  '--popover-foreground',
  '--muted',
  '--muted-foreground',
  '--secondary',
  '--secondary-foreground',
  '--accent',
  '--accent-foreground',
  '--border',
  '--input',
  '--input-panel-bg',
  '--input-panel-bg-hover',
  '--input-panel-border',
  '--input-panel-action-bg',
  '--input-panel-action-bg-hover',
  '--input-panel-action-border',
]


export const TONE_SCALE_TOKENS = [...tone('purple'), ...tone('orange'), ...tone('slate')]

export const GENERAL_TOKEN_GROUPS: Array<{ id: TokenGroupId; tokens: string[] }> = [
  { id: 'surfaces', tokens: SURFACE_TOKENS },
  { id: 'tones', tokens: TONE_SCALE_TOKENS },
]
