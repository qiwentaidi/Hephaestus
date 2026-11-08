import { cn } from '@/lib/utils'

export interface DbConfig {
  iconPath: string
  label: string
}

export const getDbConfig = (scheme: string): DbConfig => {
  const schemeLower = scheme.toLowerCase()
  switch (schemeLower) {
    case 'mysql':
      return {
        iconPath: '/icon/mysql.svg',
        label: 'MySQL',
      }
    case 'postgres':
    case 'postgresql':
      return {
        iconPath: '/icon/postgresql.svg',
        label: 'PostgreSQL',
      }
    case 'mssql':
    case 'sqlserver':
      return {
        iconPath: '/icon/sqlserver.svg',
        label: 'SQL Server',
      }
    case 'oracle':
      return {
        iconPath: '/icon/oracle.svg',
        label: 'Oracle',
      }
    case 'mongodb':
      return {
        iconPath: '/icon/mongodb.svg',
        label: 'MongoDB',
      }
    case 'redis':
      return {
        iconPath: '/icon/redis.svg',
        label: 'Redis',
      }
    default:
      return {
        iconPath: '/icon/mysql.svg', // 默认图标
        label: scheme.toUpperCase(),
      }
  }
}

interface DatabaseIconProps {
  scheme: string
  className?: string
  size?: number
}

export function DatabaseIconComponent({ scheme, className, size = 16 }: DatabaseIconProps) {
  const dbConfig = getDbConfig(scheme)

  return (
    <div
      className="flex shrink-0 items-center justify-center rounded bg-muted p-0.5 text-foreground"
      style={{
        width: size + 4,
        height: size + 4,
        minWidth: size + 4,
        minHeight: size + 4,
      }}
    >
      <span
        role="img"
        aria-label={dbConfig.label}
        className={cn('block shrink-0 bg-current', className)}
        style={{
          width: size,
          height: size,
          maskImage: `url("${dbConfig.iconPath}")`,
          WebkitMaskImage: `url("${dbConfig.iconPath}")`,
          maskSize: 'contain',
          WebkitMaskSize: 'contain',
          maskRepeat: 'no-repeat',
          WebkitMaskRepeat: 'no-repeat',
          maskPosition: 'center',
          WebkitMaskPosition: 'center',
        }}
      />
    </div>
  )
}

export function getDbName(scheme: string) {
  return getDbConfig(scheme).label
}
