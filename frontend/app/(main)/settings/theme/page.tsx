import { ThemeEditorPage } from '../components/ThemeEditorPage'

export default function SettingsThemeEditorPage() {
  return (
    <div className="h-full min-h-0">
      <ThemeEditorPage returnPath="/settings" />
    </div>
  )
}
