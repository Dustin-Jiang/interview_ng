import { cva, type VariantProps } from 'class-variance-authority'

/**
 * 跨组件共享的视觉 token 注册表（cva 变体）。
 * 组件私有变体（button/badge 等）仍留在各自目录；此处只收编
 * 多处视图重复手写、且曾出现漂移的表面样式，保证单一实现。
 */

/** 页签/导航项：激活实底、未激活弱化（App 全局导航与 UsersView 页签共用）。 */
export const tabItemVariants = cva(
  'inline-flex cursor-pointer shrink-0 items-center rounded-md px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-lg:min-h-11',
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
 * 设置页左侧栏导航项：纵向列表式导航。
 * 激活弱化实底（区别于页顶 tab 的主色实底），全宽 + 图标 + 文字。
 */
export const navItemVariants = cva(
  'flex w-full cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-lg:min-h-11',
  {
    variants: {
      active: {
        true: 'bg-accent text-accent-foreground',
        false: 'text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground',
      },
    },
    defaultVariants: {
      active: false,
    },
  },
)

export type NavItemVariants = VariantProps<typeof navItemVariants>

/**
 * 段式选择控件项（shadcn 风格）：置于 bg-muted 轨道内，激活项以 bg-background 脱离轨道。
 * 无实底填充、无阴影，仅激活文字/底色区分（含文字标签，不依赖颜色单通道）。
 * 行内 flex：项内可能是「文字 + 键帽」（如录取决定的 1/2/3 快捷键提示）。
 */
export const segmentedItemVariants = cva(
  'inline-flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-lg:min-h-11',
  {
    variants: {
      active: {
        true: 'bg-background text-foreground',
        false: 'text-muted-foreground hover:text-foreground',
      },
    },
    defaultVariants: {
      active: false,
    },
  },
)

export type SegmentedItemVariants = VariantProps<typeof segmentedItemVariants>

/**
 * 可交互卡片表面：房间卡片、快捷入口等点击式 tile 的统一外观。
 * interactive 态统一提供 hover 反馈与 focus-within 焦点环。
 */
export const tileVariants = cva(
  'rounded-xl border bg-card transition-colors',
  {
    variants: {
      interactive: {
        true: 'hover:bg-accent/40 focus-within:ring-2 focus-within:ring-ring',
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
      /**
       * 同一位发送者的连续消息（气泡组）：上方还有同组气泡时收紧「自己这一侧」的上圆角，
       * 让堆叠的气泡连成一片（与 shadcn `BubbleGroup` 同一手法）。
       */
      grouped: {
        true: '',
        false: '',
      },
    },
    compoundVariants: [
      { grouped: true, side: 'own', class: 'rounded-tr-md' },
      { grouped: true, side: 'other', class: 'rounded-tl-md' },
    ],
    defaultVariants: {
      side: 'other',
      grouped: false,
    },
  },
)

export type ChatBubbleVariants = VariantProps<typeof chatBubbleVariants>
