export type RiskLevel = 'critical' | 'high' | 'medium' | 'low' | 'info' | 'none'

export function getRiskLevelBadgeClassName(level: RiskLevel): string {
	switch (level) {
		case 'critical':
			return 'text-tone-red bg-tone-red-soft border-tone-red-border'
		case 'high':
			return 'text-tone-orange bg-tone-orange-soft border-tone-orange-border'
		case 'medium':
			return 'text-tone-amber bg-tone-amber-soft border-tone-amber-border'
		case 'low':
			return 'text-tone-blue bg-tone-blue-soft border-tone-blue-border'
		case 'info':
			return 'text-tone-slate bg-tone-slate-soft border-tone-slate-border'
		default:
			return 'text-tone-slate bg-tone-slate-soft border-tone-slate-border'
	}
}
