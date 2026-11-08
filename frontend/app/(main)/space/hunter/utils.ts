import type { HunterResult } from './types'

export const getResultMessage = (message?: string, _total?: number, restQuota?: string) => {
	const parts: string[] = []
	if (message) {
		parts.push(message)
	}
	if (restQuota) {
		parts.push(restQuota)
	}
	return parts.length > 0 ? parts.join(',') : '未查询到数据'
}

export const transformHunterData = (arr: any[]): HunterResult[] => {
	return arr.map((item: any) => {
		let component = ''
		if (item.component) {
			if (Array.isArray(item.component)) {
				component = item.component
					.map((c: any) => {
						if (typeof c === 'string') return c
						return `${c.name || ''}${c.version ? ' ' + c.version : ''}`.trim()
					})
					.filter(Boolean)
					.join(', ')
			} else {
				component = String(item.component)
			}
		}

		let position = ''
		if (item.country || item.province) {
			const parts = [item.country, item.province].filter(Boolean)
			position = parts.join('/')
		}

		return {
			URL: item.url || '',
			IP: item.ip || '',
			Port: item.port || 0,
			Protocol: item.protocol || '',
			Domain: item.domain || '',
			Component: component,
			Title: item.web_title || '',
			Status: item.status_code || 0,
			ICP: item.company || '',
			ISP: item.isp || '',
			Position: position,
			UpdateTime: item.updated_at || '',
		}
	})
}

export const formatProduct = (product: string): string[] => {
	if (!product || !product.trim()) return []
	return product
		.split(',')
		.map(p => p.trim())
		.filter(p => p.length > 0)
}
