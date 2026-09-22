import { cva, type VariantProps } from 'class-variance-authority'

export { default as Button } from './Button.vue'

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:bg-primary/90',
        destructive:
          'bg-destructive text-destructive-foreground hover:bg-destructive/90',
        outline:
          'border border-input bg-background hover:bg-accent hover:text-accent-foreground',
        secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
        ghost: 'hover:bg-accent hover:text-accent-foreground',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        // 触屏抬高到 ≥44px（触控目标下限）：按 ≤lg 判断而非手机——平板竖屏（768~1023）同样是触屏；
        // 桌面（鼠标、≥lg）保持原有紧凑尺寸。
        default: 'h-9 px-4 py-2 max-lg:h-11',
        sm: 'h-8 rounded-md px-3 text-xs max-lg:h-11 max-lg:px-4 max-lg:text-sm',
        lg: 'h-10 rounded-md px-8 max-lg:h-12',
        icon: 'h-9 w-9 max-lg:h-11 max-lg:w-11',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
)

export type ButtonVariants = VariantProps<typeof buttonVariants>
