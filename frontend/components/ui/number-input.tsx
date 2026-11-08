import * as React from 'react'
import { ChevronDownIcon, ChevronUpIcon } from 'lucide-react'

import { cn } from '@/lib/utils'

type NumberInputProps = Omit<
  React.ComponentProps<'input'>,
  'type'
> & {
  value?: number | string
  defaultValue?: number | string
  onValueChange?: (value: number) => void
  step?: number
  inputClassName?: string
}

const normalizeBound = (bound: React.ComponentProps<'input'>['min']) => {
  if (bound === undefined || bound === '') {
    return undefined
  }

  const numericBound = Number(bound)
  return Number.isFinite(numericBound) ? numericBound : undefined
}

function NumberInput({
  className,
  inputClassName,
  value,
  defaultValue,
  onChange,
  onBlur,
  onFocus,
  onKeyDown,
  onValueChange,
  min,
  max,
  step = 1,
  disabled,
  ...props
}: NumberInputProps) {
  const isControlled = value !== undefined
  const [isEditing, setIsEditing] = React.useState(false)
  const [internalValue, setInternalValue] = React.useState<number | string | undefined>(
    defaultValue,
  )
  const committedValue = isControlled ? value : internalValue
  const [draftValue, setDraftValue] = React.useState(() =>
    committedValue === undefined ? '' : String(committedValue),
  )
  const numericMin = React.useMemo(() => normalizeBound(min), [min])
  const numericMax = React.useMemo(() => normalizeBound(max), [max])

  const clampValue = React.useCallback(
    (next: number) => {
      let result = next

      if (numericMin !== undefined) {
        result = Math.max(numericMin, result)
      }
      if (numericMax !== undefined) {
        result = Math.min(numericMax, result)
      }

      return result
    },
    [numericMax, numericMin],
  )

  const getFallbackValue = React.useCallback(
    () =>
      numericMin !== undefined
        ? numericMin
        : numericMax !== undefined && numericMax < 0
          ? numericMax
          : 0,
    [numericMax, numericMin],
  )

  const commitValue = React.useCallback(
    (next: number) => {
      const normalized = Number.isFinite(next)
        ? clampValue(next)
        : getFallbackValue()

      setDraftValue(String(normalized))
      if (!isControlled) {
        setInternalValue(normalized)
      }
      onValueChange?.(normalized)

      if (onChange) {
        const syntheticEvent = {
          target: { value: String(normalized) },
          currentTarget: { value: String(normalized) },
        } as React.ChangeEvent<HTMLInputElement>
        onChange(syntheticEvent)
      }
    },
    [clampValue, getFallbackValue, isControlled, onChange, onValueChange],
  )

  const commitDraftValue = React.useCallback(
    () => {
      const rawValue = draftValue.trim()
      commitValue(rawValue === '' ? Number.NaN : Number(rawValue))
    },
    [commitValue, draftValue],
  )

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const rawValue = event.target.value
    setDraftValue(rawValue)
    if (!isControlled) {
      setInternalValue(rawValue)
    }
    onChange?.(event)

    if (rawValue !== '') {
      const numericValue = Number(rawValue)
      if (Number.isFinite(numericValue)) {
        onValueChange?.(numericValue)
      }
    }
  }

  const handleInputFocus = (event: React.FocusEvent<HTMLInputElement>) => {
    setIsEditing(true)
    setDraftValue(committedValue === undefined ? '' : String(committedValue))
    onFocus?.(event)
  }

  const handleInputBlur = (event: React.FocusEvent<HTMLInputElement>) => {
    commitDraftValue()
    setIsEditing(false)
    onBlur?.(event)
  }

  const handleInputKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    onKeyDown?.(event)
    if (!event.defaultPrevented && event.key === 'Enter') {
      commitDraftValue()
    }
  }

  const adjustValue = (direction: 1 | -1) => {
    const currentValue = isEditing ? draftValue : committedValue
    const numericValue =
      typeof currentValue === 'string' ? Number(currentValue) : currentValue
    const numericDefaultValue =
      typeof defaultValue === 'string' ? Number(defaultValue) : defaultValue
    const baseValue = Number.isFinite(numericValue)
      ? Number(numericValue)
      : Number.isFinite(numericDefaultValue)
        ? Number(numericDefaultValue)
        : 0
    commitValue(baseValue + direction * step)
  }

  return (
    <div className={cn('relative w-full', className)}>
      <input
        {...props}
        type="number"
        inputMode="numeric"
        data-slot="number-input-field"
        disabled={disabled}
        min={min}
        max={max}
        step={step}
        value={isEditing ? draftValue : committedValue ?? ''}
        onChange={handleInputChange}
        onFocus={handleInputFocus}
        onBlur={handleInputBlur}
        onKeyDown={handleInputKeyDown}
        className={cn(
          'file:text-foreground placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground dark:bg-input/30 border-input h-9 w-full min-w-0 rounded-md border bg-transparent px-3 py-1 pr-10 text-base shadow-xs transition-[color,box-shadow] outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
          'focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]',
          'aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive',
          inputClassName,
        )}
      />
      <div className="absolute inset-y-1 right-1 flex w-7 flex-col items-center justify-center">
        <button
          type="button"
          tabIndex={-1}
          aria-label="Increase value"
          disabled={disabled}
          onClick={() => adjustValue(1)}
          className="text-muted-foreground hover:text-foreground flex h-[45%] w-full items-end justify-center transition-colors disabled:pointer-events-none"
        >
          <ChevronUpIcon className="size-3" />
        </button>
        <button
          type="button"
          tabIndex={-1}
          aria-label="Decrease value"
          disabled={disabled}
          onClick={() => adjustValue(-1)}
          className="text-muted-foreground hover:text-foreground flex h-[45%] w-full items-start justify-center transition-colors disabled:pointer-events-none"
        >
          <ChevronDownIcon className="size-3" />
        </button>
      </div>
    </div>
  )
}

export { NumberInput }
