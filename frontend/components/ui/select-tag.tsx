'use client'

import { useMemo } from 'react'
import { ChevronsUpDown, X } from 'lucide-react'
import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxValue,
  useComboboxAnchor,
} from '@/components/ui/combobox'
import { cn } from '@/lib/utils'

export interface SelectTagOption {
	value: string
	label: string
}

interface SelectTagProps {
	options: SelectTagOption[]
	value: string[]
	onChange: (next: string[]) => void
	placeholder?: string
	searchPlaceholder?: string
	emptyText?: string
	disabled?: boolean
}

export function SelectTag({
	options,
	value,
	onChange,
	placeholder = '请选择',
	searchPlaceholder = '搜索字段...',
	emptyText = '无匹配字段',
	disabled = false,
}: SelectTagProps) {
	const anchor = useComboboxAnchor()

	// 合并 options 与已选值，避免出现“已选字段在下拉里找不到”
	const mergedOptions = useMemo(() => {
		const byValue = new Map<string, SelectTagOption>()
		options.forEach(option => byValue.set(option.value, option))
		value.forEach(itemValue => {
			if (!byValue.has(itemValue)) {
				byValue.set(itemValue, { value: itemValue, label: itemValue })
			}
		})
		return Array.from(byValue.values())
	}, [options, value])

	const removeValue = (itemValue: string) => {
		onChange(value.filter(v => v !== itemValue))
	}

	const selectedOptions = useMemo(
		() => mergedOptions.filter((item) => value.includes(item.value)),
		[mergedOptions, value],
	)

	return (
		<Combobox<SelectTagOption>
			multiple
			items={mergedOptions}
			value={selectedOptions}
			onValueChange={(next) => {
				const list = Array.isArray(next) ? next : next ? [next] : []
				onChange(list.map((item) => item.value))
			}}
			disabled={disabled}
		>
			<ComboboxChips
				ref={anchor}
				className={cn(
					'w-full min-h-10 rounded-md border bg-background px-3 py-2 text-sm flex items-center gap-2 flex-wrap',
					'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
					disabled && 'cursor-not-allowed opacity-50',
				)}
			>
				<ComboboxValue<SelectTagOption>>
					{(values) => {
						const list = Array.isArray(values) ? values : values ? [values] : []
						if (list.length === 0) {
							return <span className="text-muted-foreground">{placeholder}</span>
						}
						return (
							<>
								{list.map((option) => (
									<ComboboxChip key={option.value} className="max-w-full">
										<span className="truncate">{option.label}</span>
										<span
											data-combobox-stop-open="true"
											role="button"
											tabIndex={disabled ? -1 : 0}
											aria-label={`移除 ${option.label || option.value}`}
											onPointerDown={(event) => {
												event.preventDefault()
												event.stopPropagation()
											}}
											onMouseDown={(event) => {
												event.preventDefault()
												event.stopPropagation()
											}}
											onClick={(event) => {
												event.preventDefault()
												event.stopPropagation()
												if (!disabled) {
													removeValue(option.value)
												}
											}}
											onKeyDown={(event) => {
												if ((event.key === 'Enter' || event.key === ' ') && !disabled) {
													event.preventDefault()
													removeValue(option.value)
												}
											}}
											className="cursor-pointer hover:text-destructive"
										>
											<X className="w-3 h-3" />
										</span>
									</ComboboxChip>
								))}
							</>
						)
					}}
				</ComboboxValue>
				<div className="ml-auto flex items-center gap-1">
					{value.length > 0 && !disabled && (
						<span
							data-combobox-stop-open="true"
							role="button"
							tabIndex={0}
							aria-label="清空已选"
							onPointerDown={(event) => {
								event.preventDefault()
								event.stopPropagation()
							}}
							onMouseDown={(event) => {
								event.preventDefault()
								event.stopPropagation()
							}}
							onClick={(event) => {
								event.preventDefault()
								event.stopPropagation()
								onChange([])
							}}
							onKeyDown={(event) => {
								if (event.key === 'Enter' || event.key === ' ') {
									event.preventDefault()
									onChange([])
								}
							}}
							className="text-muted-foreground hover:text-destructive"
						>
							<X className="h-4 w-4" />
						</span>
					)}
					<ChevronsUpDown className="h-4 w-4 shrink-0 opacity-50" />
				</div>
			</ComboboxChips>
			<ComboboxContent<SelectTagOption> anchor={anchor} className="w-[var(--radix-popover-trigger-width)]">
				<div className="border-b p-2">
					<ComboboxInput<SelectTagOption>
						placeholder={searchPlaceholder}
						showTrigger={false}
					/>
				</div>
				<ComboboxEmpty<SelectTagOption>>{emptyText}</ComboboxEmpty>
				<ComboboxList<SelectTagOption>>
					{(option) => (
						<ComboboxItem key={option.value} value={option}>
							{option.label}
						</ComboboxItem>
					)}
				</ComboboxList>
			</ComboboxContent>
		</Combobox>
	)
}
