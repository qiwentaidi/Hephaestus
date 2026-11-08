import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { TextInputWithActions } from '@/components/ui/text-input-with-actions'
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select'
import { useI18n } from '@/lib/i18n'
import { X } from 'lucide-react'

interface CrackScanSettingsProps {
    username: string
    setUsername: (val: string) => void
    password: string
    setPassword: (val: string) => void
    defaultDict: boolean
    setDefaultDict: (val: boolean) => void
    isSubmitting: boolean
    // 模板相关
    scanTemplates?: any[]
    selectedTemplateId?: string
    setSelectedTemplateId?: (val: string) => void
    onApplyTemplate?: (id: string) => void
    onClearTemplate?: () => void
}

export function CrackScanSettings({
    username,
    setUsername,
    password,
    setPassword,
    defaultDict,
    setDefaultDict,
    isSubmitting,
    scanTemplates = [],
    selectedTemplateId = '',
    setSelectedTemplateId,
    onApplyTemplate,
    onClearTemplate,
}: CrackScanSettingsProps) {
    const { t } = useI18n()

    return (
        <div className="space-y-6 pt-4 border-t">
            {/* 模板选择 */}
            {scanTemplates.length > 0 && (
                <div className="space-y-2">
                    <Label>{t.vulnScan.newScan.applyTemplate}</Label>
                    <div className="flex items-center gap-2">
                        <Select
                            value={selectedTemplateId}
                            onValueChange={(value) => {
                                setSelectedTemplateId?.(value)
                                if (value && onApplyTemplate) {
                                    onApplyTemplate(value)
                                }
                            }}
                        >
                            <SelectTrigger className="flex-1">
                                <SelectValue placeholder={t.vulnScan.newScan.selectTemplate} />
                            </SelectTrigger>
                            <SelectContent>
                                {scanTemplates.map((template) => (
                                    <SelectItem key={template.id} value={template.id}>
                                        {template.name}
                                        {template.description && (
                                            <span className="text-muted-foreground ml-2">
                                                - {template.description}
                                            </span>
                                        )}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                        {selectedTemplateId && (
                            <Button
                                type="button"
                                variant="outline"
                                size="icon"
                                onClick={onClearTemplate}
                                title={t.vulnScan.newScan.clearTemplate}
                            >
                                <X className="h-4 w-4" />
                            </Button>
                        )}
                    </div>
                </div>
            )}

            {/* 用户字典 */}
            <div className="space-y-2">
                <Label>{t.vulnScan.newScan.userDict}</Label>
                <TextInputWithActions
                    value={username}
                    onChange={(value) => {
                        setUsername(value)
                        if (value.length === 0) {
                            setDefaultDict(true)
                        }
                    }}
                    placeholder={t.vulnScan.newScan.userDictPlaceholder}
                    disabled={isSubmitting}
                    height="120px"
                />
            </div>

            {/* 密码字典 */}
            <div className="space-y-2">
                <Label>{t.vulnScan.newScan.passwordDict}</Label>
                <TextInputWithActions
                    value={password}
                    onChange={(value) => {
                        setPassword(value)
                        if (value.length === 0) {
                            setDefaultDict(true)
                        }
                    }}
                    placeholder={t.vulnScan.newScan.passwordDictPlaceholder}
                    disabled={isSubmitting}
                    height="120px"
                />
                <div className="flex items-center space-x-2">
                    <Checkbox
                        id="useSystemDict"
                        checked={defaultDict}
                        onCheckedChange={(checked) =>
                            setDefaultDict(checked === true)
                        }
                        disabled={password.length > 0}
                    />
                    <Label
                        htmlFor="useSystemDict"
                        className="text-sm font-normal cursor-pointer"
                    >
                        {t.vulnScan.newScan.useDefaultDict}
                    </Label>
                </div>
            </div>
        </div>
    )
}
