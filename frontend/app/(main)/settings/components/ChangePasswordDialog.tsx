'use client'

import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { KeyRound, AlertCircle, Check } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import { toast } from 'sonner'
import { useI18n } from '@/lib/i18n'

interface ChangePasswordDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ChangePasswordDialog({ open, onOpenChange }: ChangePasswordDialogProps) {
  const { t } = useI18n()
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState('')
  const [formData, setFormData] = useState({
    oldPassword: '',
    newPassword: '',
    confirmPassword: '',
  })

  // 验证密码复杂性
  const validatePassword = (password: string): { valid: boolean; message: string } => {
    if (password.length < 8) {
      return { valid: false, message: t.changePasswordDialog.validation.minLength }
    }
    if (!/[A-Z]/.test(password)) {
      return { valid: false, message: t.changePasswordDialog.validation.uppercase }
    }
    if (!/[a-z]/.test(password)) {
      return { valid: false, message: t.changePasswordDialog.validation.lowercase }
    }
    if (!/[0-9]/.test(password)) {
      return { valid: false, message: t.changePasswordDialog.validation.number }
    }
    return { valid: true, message: '' }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    // 验证新密码复杂性
    const validation = validatePassword(formData.newPassword)
    if (!validation.valid) {
      setError(validation.message)
      return
    }

    if (formData.newPassword !== formData.confirmPassword) {
      setError(t.changePasswordDialog.validation.confirmMismatch)
      return
    }

    if (formData.oldPassword === formData.newPassword) {
      setError(t.changePasswordDialog.validation.sameAsOld)
      return
    }

    setIsLoading(true)

    try {
      const userData = localStorage.getItem('userData')
      const username = userData ? JSON.parse(userData).username : ''

      const response = await fetchWithAuth(`/api/v1/auth/password`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-Username': username,
        },
        body: JSON.stringify({
          oldPassword: formData.oldPassword,
          newPassword: formData.newPassword,
        }),
      })

      const data = await response.json()

      if (data.success) {
        toast.success(t.changePasswordDialog.success)
        onOpenChange(false)
        // 重置表单
        setFormData({
          oldPassword: '',
          newPassword: '',
          confirmPassword: '',
        })
      } else {
        setError(data.error || t.changePasswordDialog.updateFailed)
      }
    } catch (error) {
      console.error('修改密码请求失败:', error)
      setError(t.changePasswordDialog.requestFailed)
    } finally {
      setIsLoading(false)
    }
  }

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      // 关闭时重置表单
      setFormData({
        oldPassword: '',
        newPassword: '',
        confirmPassword: '',
      })
      setError('')
    }
    onOpenChange(open)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="w-5 h-5" />
            {t.changePasswordDialog.title}
          </DialogTitle>
          <DialogDescription>
            {t.changePasswordDialog.description}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="old-password">{t.changePasswordDialog.oldPassword}</Label>
              <Input
                id="old-password"
                type="password"
                placeholder={t.changePasswordDialog.oldPasswordPlaceholder}
                value={formData.oldPassword}
                onChange={(e) => setFormData({ ...formData, oldPassword: e.target.value })}
                required
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="new-password">{t.changePasswordDialog.newPassword}</Label>
              <Input
                id="new-password"
                type="password"
                placeholder={t.changePasswordDialog.newPasswordPlaceholder}
                value={formData.newPassword}
                onChange={(e) => setFormData({ ...formData, newPassword: e.target.value })}
                required
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="confirm-password">{t.changePasswordDialog.confirmPassword}</Label>
              <Input
                id="confirm-password"
                type="password"
                placeholder={t.changePasswordDialog.confirmPasswordPlaceholder}
                value={formData.confirmPassword}
                onChange={(e) => setFormData({ ...formData, confirmPassword: e.target.value })}
                required
              />
            </div>
            {error && (
              <div className="flex items-start gap-2 p-3 rounded-lg bg-destructive/10 border border-destructive/20">
                <AlertCircle className="w-4 h-4 text-destructive shrink-0 mt-0.5" />
                <p className="text-sm text-destructive">{error}</p>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              {t.changePasswordDialog.cancel}
            </Button>
            <Button type="submit" disabled={isLoading}>
              {isLoading ? (
                <span className="flex items-center gap-2">
                  <span className="w-4 h-4 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
                  {t.changePasswordDialog.submitting}
                </span>
              ) : (
                <span className="flex items-center gap-2">
                  <Check className="w-4 h-4" />
                  {t.changePasswordDialog.submit}
                </span>
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
