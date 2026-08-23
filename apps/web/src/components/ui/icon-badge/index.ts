import { cva } from 'class-variance-authority'

export { default as IconBadge } from './IconBadge.vue'

/** 变体定义置于 index.ts（与 button/badge 约定一致），供组件与外部复用。 */
export const iconBadgeVariants = cva(
  'flex shrink-0 items-center justify-center rounded-full',
  {
    variants: {
      /** 色调：primary 弱化底、destructive 弱化底、solid 实底（品牌）。 */
      tone: {
        primary: 'bg-primary/10 text-primary',
        destructive: 'bg-destructive/10 text-destructive',
        solid: 'bg-primary text-primary-foreground',
      },
      size: {
        sm: 'h-8 w-8 [&_svg]:size-4',
        md: 'h-10 w-10 [&_svg]:size-5',
        lg: 'h-12 w-12 [&_svg]:size-6',
      },
    },
    defaultVariants: {
      tone: 'primary',
      size: 'md',
    },
  },
)
