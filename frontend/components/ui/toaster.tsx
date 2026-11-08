'use client'

import { Toaster, ToastBar } from 'react-hot-toast'
import toast from '@/lib/toast'

export function AppToaster() {
  return (
    <Toaster 
      position="bottom-right"
      reverseOrder={false}
      gutter={8}
      containerStyle={{
        bottom: 20,
        right: 20,
      }}
      toastOptions={{
        duration: 5000,
        ariaProps: {
          role: 'status',
          'aria-live': 'polite',
        },
        style: {
          background: 'var(--card)',
          color: 'var(--card-foreground)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius)',
          padding: '14px 16px',
          fontSize: '14px',
          fontWeight: '400',
          lineHeight: '1.5',
          boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05)',
          maxWidth: '420px',
          minWidth: '300px',
        },
        success: {
          iconTheme: {
            primary: 'var(--primary)',
            secondary: 'var(--primary-foreground)',
          },
          style: {
            background: 'var(--card)',
            color: 'var(--card-foreground)',
            border: '1px solid var(--border)',
            borderLeft: '4px solid var(--primary)',
            boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05), 0 0 20px rgba(255, 120, 50, 0.1)',
          },
        },
        error: {
          iconTheme: {
            primary: 'var(--destructive)',
            secondary: 'var(--destructive-foreground)',
          },
          style: {
            background: 'var(--card)',
            color: 'var(--card-foreground)',
            border: '1px solid var(--border)',
            borderLeft: '4px solid var(--destructive)',
            boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05), 0 0 20px rgba(255, 80, 50, 0.15)',
          },
        },
        loading: {
          iconTheme: {
            primary: 'var(--accent)',
            secondary: 'var(--accent-foreground)',
          },
          style: {
            background: 'var(--card)',
            color: 'var(--card-foreground)',
            border: '1px solid var(--border)',
            borderLeft: '4px solid var(--accent)',
            boxShadow: '0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05)',
          },
        },
      }}
    >
      {(t) => (
        <ToastBar toast={t}>
          {({ icon, message }) => (
            <div style={{ 
              display: 'flex', 
              alignItems: 'flex-start', 
              gap: '12px',
              width: '100%',
              minWidth: 0,
            }}>
              {icon && (
                <div style={{ 
                  flexShrink: 0,
                  marginTop: '2px',
                }}>
                  {icon}
                </div>
              )}
              <div style={{ 
                flex: 1,
                minWidth: 0,
                display: 'flex',
                flexDirection: 'column',
              }}>
                {message}
              </div>
              {t.type !== 'loading' && (
                <button
                  onClick={() => toast.dismiss(t.id)}
                  style={{
                    background: 'transparent',
                    border: 'none',
                    color: 'var(--muted-foreground)',
                    cursor: 'pointer',
                    padding: '4px',
                    marginLeft: '4px',
                    borderRadius: '4px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    opacity: 0.7,
                    transition: 'opacity 0.2s, color 0.2s',
                    fontSize: '18px',
                    lineHeight: '1',
                    flexShrink: 0,
                    width: '20px',
                    height: '20px',
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.opacity = '1';
                    e.currentTarget.style.color = 'var(--foreground)';
                    e.currentTarget.style.background = 'var(--muted)';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.opacity = '0.7';
                    e.currentTarget.style.color = 'var(--muted-foreground)';
                    e.currentTarget.style.background = 'transparent';
                  }}
                  aria-label="Close"
                >
                  ×
                </button>
              )}
            </div>
          )}
        </ToastBar>
      )}
    </Toaster>
  )
}

// 导出 toast 以便全局使用
export { toast }

// 导出扩展的 toast 工具（包含 warning 和 info，支持 title 和 message）
export { default as toastUtils } from '@/lib/toast'
