'use client'

import toast from 'react-hot-toast'
import React from 'react'
import { CheckCircle2, XCircle, Loader2, AlertTriangle, Info } from 'lucide-react'

// Toast 选项类型
export interface ToastOptions {
  title?: string
  message?: string
  duration?: number
  [key: string]: any
}

// Toast 内容组件
const ToastContent: React.FC<{ title?: string; message?: string }> = ({ title, message }) => {
  if (title && message) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', width: '100%' }}>
        <div style={{ 
          fontWeight: '600', 
          fontSize: '14px', 
          lineHeight: '1.4',
          color: 'var(--foreground)',
        }}>
          {title}
        </div>
        <div style={{ 
          fontSize: '13px', 
          lineHeight: '1.5', 
          color: 'var(--muted-foreground)',
          wordBreak: 'break-word',
        }}>
          {message}
        </div>
      </div>
    )
  }
  if (title) {
    return (
      <div style={{ 
        fontWeight: '600', 
        fontSize: '14px', 
        lineHeight: '1.4',
        color: 'var(--foreground)',
      }}>
        {title}
      </div>
    )
  }
  if (message) {
    return (
      <div style={{ 
        fontSize: '14px', 
        lineHeight: '1.5',
        color: 'var(--foreground)',
        wordBreak: 'break-word',
      }}>
        {message}
      </div>
    )
  }
  return null
}

// 解析消息：支持字符串（可能包含 \n）或对象（title + message）
const parseMessage = (input: string | ToastOptions): { title?: string; message?: string } => {
  if (typeof input === 'string') {
    // 如果包含 \n，分割为标题和消息
    const parts = input.split('\n')
    if (parts.length > 1) {
      return {
        title: parts[0],
        message: parts.slice(1).join('\n'),
      }
    }
    return { message: input }
  }
  // 如果是对象，直接返回
  return {
    title: input.title,
    message: input.message,
  }
}

// 图标组件
const SuccessIcon = () => (
  <CheckCircle2 
    className="w-5 h-5" 
    style={{ color: 'var(--primary)' }}
  />
)

const ErrorIcon = () => (
  <XCircle 
    className="w-5 h-5" 
    style={{ color: 'var(--destructive)' }}
  />
)

const LoadingIcon = () => (
  <Loader2 
    className="w-5 h-5 animate-spin" 
    style={{ color: 'var(--accent)' }}
  />
)

const WarningIcon = () => (
  <AlertTriangle 
    className="w-5 h-5" 
    style={{ color: 'oklch(0.70 0.20 80)' }}
  />
)

const InfoIcon = () => (
  <Info 
    className="w-5 h-5" 
    style={{ color: 'oklch(0.60 0.15 240)' }}
  />
)

// 扩展 toast 对象
const toastWithExtras = {
  ...toast,
  
  // 成功提示
  success: (input: string | ToastOptions, options?: Parameters<typeof toast>[1]) => {
    const { title, message } = parseMessage(input)
    return toast(
      () => <ToastContent title={title} message={message} />,
      {
        ...options,
        icon: <SuccessIcon />,
        duration: options?.duration || 5000,
        style: {
          background: 'var(--card)',
          color: 'var(--card-foreground)',
          border: '1px solid var(--border)',
          borderLeft: '4px solid var(--primary)',
          borderRadius: 'var(--radius)',
          padding: '14px 16px',
          fontSize: '14px',
          fontWeight: '400',
          lineHeight: '1.5',
          boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05), 0 0 20px rgba(255, 120, 50, 0.1)',
          ...options?.style,
        },
      }
    )
  },
  
  // 错误提示
  error: (input: string | ToastOptions, options?: Parameters<typeof toast>[1]) => {
    const { title, message } = parseMessage(input)
    return toast.error(
      () => <ToastContent title={title} message={message} />,
      {
        ...options,
        icon: <ErrorIcon />,
        duration: options?.duration || 5000,
        style: {
          background: 'var(--card)',
          color: 'var(--card-foreground)',
          border: '1px solid var(--border)',
          borderLeft: '4px solid var(--destructive)',
          borderRadius: 'var(--radius)',
          padding: '14px 16px',
          fontSize: '14px',
          fontWeight: '400',
          lineHeight: '1.5',
          boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05), 0 0 20px rgba(255, 80, 50, 0.15)',
          ...options?.style,
        },
      }
    )
  },
  
  // 加载提示
  loading: (input: string | ToastOptions, options?: Parameters<typeof toast>[1]) => {
    const { title, message } = parseMessage(input)
    return toast.loading(
      () => <ToastContent title={title} message={message} />,
      {
        ...options,
        icon: <LoadingIcon />,
        style: {
          background: 'var(--card)',
          color: 'var(--card-foreground)',
          border: '1px solid var(--border)',
          borderLeft: '4px solid var(--accent)',
          borderRadius: 'var(--radius)',
          padding: '14px 16px',
          fontSize: '14px',
          fontWeight: '400',
          lineHeight: '1.5',
          boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05)',
          ...options?.style,
        },
      }
    )
  },
  
  // 警告提示
  warning: (input: string | ToastOptions, options?: Parameters<typeof toast>[1]) => {
    const { title, message } = parseMessage(input)
    return toast(
      () => <ToastContent title={title} message={message} />,
      {
        ...options,
        icon: <WarningIcon />,
        duration: options?.duration || 5000,
        style: {
          background: 'var(--card)',
          color: 'var(--card-foreground)',
          border: '1px solid var(--border)',
          borderLeft: '4px solid oklch(0.70 0.20 80)', // 黄色警告色
          borderRadius: 'var(--radius)',
          padding: '14px 16px',
          fontSize: '14px',
          fontWeight: '400',
          lineHeight: '1.5',
          boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05), 0 0 20px rgba(255, 200, 50, 0.1)',
          ...options?.style,
        },
      }
    )
  },
  
  // 信息提示
  info: (input: string | ToastOptions, options?: Parameters<typeof toast>[1]) => {
    const { title, message } = parseMessage(input)
    return toast(
      () => <ToastContent title={title} message={message} />,
      {
        ...options,
        icon: <InfoIcon />,
        duration: options?.duration || 5000,
        style: {
          background: 'var(--card)',
          color: 'var(--card-foreground)',
          border: '1px solid var(--border)',
          borderLeft: '4px solid oklch(0.60 0.15 240)', // 蓝色信息色
          borderRadius: 'var(--radius)',
          padding: '14px 16px',
          fontSize: '14px',
          fontWeight: '400',
          lineHeight: '1.5',
          boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05)',
          ...options?.style,
        },
      }
    )
  },
}

export default toastWithExtras

