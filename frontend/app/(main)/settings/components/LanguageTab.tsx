'use client'

import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Languages } from 'lucide-react'
import { useI18n, type Language } from '@/lib/i18n'
import { toast } from 'react-hot-toast'

const languages: {
  id: Language
  name: string
  localName: string
}[] = [
  {
    id: 'zh',
    name: '简体中文',
    localName: '简体中文',
  },
  {
    id: 'en',
    name: 'English',
    localName: '英语',
  },
]

export function LanguageTab() {
  const { language, setLanguage, t } = useI18n()

  const handleLanguageChange = (lang: Language) => {
    setLanguage(lang)
    toast.success(t.language.changeSuccess)
  }

  const currentLanguage = languages.find((l) => l.id === language)

  return (
    <div className="space-y-6">
      <div className="space-y-8 max-w-2xl">
        <div className="space-y-4">
          <Label className="text-base font-semibold">{t.language.title}</Label>

          <div className="space-y-3">
            <Select value={language} onValueChange={(value) => handleLanguageChange(value as Language)}>
              <SelectTrigger className="w-full max-w-md">
                <div className="flex items-center gap-2">
                  <Languages className="w-4 h-4 text-muted-foreground" />
                  <SelectValue>{currentLanguage?.name}</SelectValue>
                </div>
              </SelectTrigger>
              <SelectContent>
                {languages.map((lang) => (
                  <SelectItem key={lang.id} value={lang.id}>
                    <div className="flex flex-col">
                      <span className="font-medium">{lang.name}</span>
                      <span className="text-xs text-muted-foreground">{lang.localName}</span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t.language.description}
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
