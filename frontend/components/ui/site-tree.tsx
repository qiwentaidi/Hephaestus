'use client'

import { useState, useCallback, useImperativeHandle, forwardRef } from 'react'
import { ChevronRight, ChevronDown, Folder, FolderOpen, FileCode, Globe, Link, Image as ImageIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

// 后端返回的 ElTreeNode 结构
interface ElTreeNode {
  id: string
  label: string
  children?: ElTreeNode[]
}

// 判断是否为 URL
function isUrl(str: string): boolean {
  return /^https?:\/\//i.test(str) || str.startsWith('/')
}

// 判断是否为图片文件
function isImageFile(str: string): boolean {
  return /\.(ico|png|jpg|jpeg|gif|svg|webp|bmp|tiff?)$/i.test(str)
}

interface SiteTreeItemProps {
  node: ElTreeNode
  level: number
  isRoot?: boolean
  expandedKeys: Set<string>
  onToggle: (key: string) => void
}

function SiteTreeItem({ node, level, isRoot = false, expandedKeys, onToggle }: SiteTreeItemProps) {
  const hasChildren = node.children && node.children.length > 0
  const nodeKey = node.id || node.label
  const isExpanded = expandedKeys.has(nodeKey)

  // 根据节点层级和内容判断图标
  const getIcon = () => {
    // 根目标输入 URL 不显示图标
    if (isRoot) {
      return null
    }
    // 判断是否为 URL 类型的节点
    if (isUrl(node.label)) {
      return <Link className="h-4 w-4 text-blue-500 shrink-0" />
    }
    // 有子节点的是目录
    if (hasChildren) {
      return isExpanded ? (
        <FolderOpen className="h-4 w-4 text-yellow-500 shrink-0" />
      ) : (
        <Folder className="h-4 w-4 text-yellow-500 shrink-0" />
      )
    }
    // 图片文件
    if (isImageFile(node.label)) {
      return <ImageIcon className="h-4 w-4 text-green-500 shrink-0" />
    }
    // 叶子节点是文件/端点
    return <FileCode className="h-4 w-4 text-muted-foreground shrink-0" />
  }

  const handleToggle = () => {
    if (hasChildren) {
      onToggle(nodeKey)
    }
  }

  return (
    <div>
      <div
        className={cn(
          'flex min-w-0 items-center gap-1 rounded-md px-2 py-1 hover:bg-muted/50 cursor-pointer group',
          isRoot && 'font-medium text-primary'
        )}
        style={{ paddingLeft: `${level * 16 + 8}px` }}
        onClick={handleToggle}
        title={node.label}
      >
        {hasChildren ? (
          <button className="p-0.5 hover:bg-muted rounded shrink-0">
            {isExpanded ? (
              <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
            )}
          </button>
        ) : (
          <span className="w-4.5 shrink-0" />
        )}
        {getIcon()}
        <span className={cn('min-w-0 flex-1 truncate text-sm', !getIcon() && 'ml-1')}>{node.label}</span>
        {hasChildren && (
          <span className="shrink-0 text-xs text-muted-foreground opacity-0 group-hover:opacity-100">
            {node.children!.length}
          </span>
        )}
      </div>
      {hasChildren && isExpanded && (
        <div>
          {node.children!.map((child, index) => (
            <SiteTreeItem
              key={child.id || `${node.id}-${index}`}
              node={child}
              level={level + 1}
              expandedKeys={expandedKeys}
              onToggle={onToggle}
            />
          ))}
        </div>
      )}
    </div>
  )
}

// 递归收集所有节点的 key
function collectAllKeys(nodes: ElTreeNode[]): string[] {
  const keys: string[] = []
  const traverse = (nodeList: ElTreeNode[]) => {
    for (const node of nodeList) {
      const key = node.id || node.label
      keys.push(key)
      if (node.children && node.children.length > 0) {
        traverse(node.children)
      }
    }
  }
  traverse(nodes)
  return keys
}

// 收集前 N 层的 key
function collectKeysUpToLevel(nodes: ElTreeNode[], maxLevel: number): string[] {
  const keys: string[] = []
  const traverse = (nodeList: ElTreeNode[], currentLevel: number) => {
    if (currentLevel > maxLevel) return
    for (const node of nodeList) {
      const key = node.id || node.label
      keys.push(key)
      if (node.children && node.children.length > 0) {
        traverse(node.children, currentLevel + 1)
      }
    }
  }
  traverse(nodes, 0)
  return keys
}

export interface SiteTreeRef {
  expandAll: () => void
  collapseAll: () => void
}

interface SiteTreeProps {
  data: ElTreeNode[]
  className?: string
  defaultExpandLevel?: number
}

export const SiteTree = forwardRef<SiteTreeRef, SiteTreeProps>(
  ({ data, className, defaultExpandLevel = 2 }, ref) => {
    // 初始展开前 N 层
    const [expandedKeys, setExpandedKeys] = useState<Set<string>>(() => {
      return new Set(collectKeysUpToLevel(data, defaultExpandLevel))
    })

    const handleToggle = useCallback((key: string) => {
      setExpandedKeys((prev) => {
        const next = new Set(prev)
        if (next.has(key)) {
          next.delete(key)
        } else {
          next.add(key)
        }
        return next
      })
    }, [])

    const expandAll = useCallback(() => {
      setExpandedKeys(new Set(collectAllKeys(data)))
    }, [data])

    const collapseAll = useCallback(() => {
      setExpandedKeys(new Set())
    }, [])

    // 暴露方法给父组件
    useImperativeHandle(ref, () => ({
      expandAll,
      collapseAll,
    }), [expandAll, collapseAll])

    if (!data || data.length === 0) {
      return null
    }

    return (
      <div className={cn('min-w-0 overflow-hidden font-mono text-sm', className)}>
        {data.map((node, index) => (
          <SiteTreeItem
            key={node.id || `root-${index}`}
            node={node}
            level={0}
            isRoot
            expandedKeys={expandedKeys}
            onToggle={handleToggle}
          />
        ))}
      </div>
    )
  }
)

SiteTree.displayName = 'SiteTree'
