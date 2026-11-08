import { UnifiedNotification } from './notifications/UnifiedNotification'

export type NotificationPlatform = 'dingtalk' | 'wechat' | 'feishu' | 'webhook' | 'jira' | 'email'

export function NotificationTab() {
  return <UnifiedNotification />
}
