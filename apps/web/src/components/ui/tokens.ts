import { cva, type VariantProps } from 'class-variance-authority'

/**
 * 跨组件共享的视觉 token 注册表（cva 变体）。
 * 组件私有变体（button/badge 等）仍留在各自目录；此处只收编
 * 多处视图重复手写、且曾出现漂移的表面样式，保证单一实现。
 */

/** 页签/导航项：激活实底、未激活弱化（App 全局导航与 UsersView 页签共用）。 */
export const tabItemVariants = cva(
  'cursor-pointer shrink-0 rounded-md px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
  {
    variants: {
      active: {
        true: 'bg-primary text-primary-foreground',
        false: 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
      },
    },
    defaultVariants: {
      active: false,
    },
  },
)

export type TabItemVariants = VariantProps<typeof tabItemVariants>

/**
 * 可交互卡片表面：房间卡片、快捷入口等点击式 tile 的统一外观。
 * interactive 态统一提供 hover 反馈与 focus-within 焦点环。
 */
export const tileVariants = cva(
  'rounded-xl border bg-card shadow-sm transition-colors',
  {
    variants: {
      interactive: {
        true: 'hover:bg-accent/40 hover:shadow focus-within:ring-2 focus-within:ring-ring',
        false: '',
      },
    },
    defaultVariants: {
      interactive: true,
    },
  },
)

export type TileVariants = VariantProps<typeof tileVariants>

/** 聊天气泡：房间实时聊天与归档回看共用的统一外观。 */
export const chatBubbleVariants = cva(
  'max-w-[85%] whitespace-pre-wrap break-words rounded-2xl px-3.5 py-2 text-sm leading-relaxed sm:max-w-[75%]',
  {
    variants: {
      side: {
        own: 'rounded-br-md bg-primary text-primary-foreground',
        other: 'rounded-bl-md bg-muted',
      },
    },
    defaultVariants: {
      side: 'other',
    },
  },
)

export type ChatBubbleVariants = VariantProps<typeof chatBubbleVariants>
