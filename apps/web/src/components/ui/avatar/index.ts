import { cva, type VariantProps } from 'class-variance-authority'

export { default as Avatar } from './AvatarRoot.vue'
export { default as AvatarImage } from './AvatarImage.vue'
export { default as AvatarFallback } from './AvatarFallback.vue'

/** 头像尺寸档（sm=32px / md=36px / lg=48px），供 AvatarRoot 传 class 使用。 */
export const avatarVariants = cva('relative flex shrink-0 select-none overflow-hidden rounded-full', {
  variants: {
    size: {
      sm: 'h-8 w-8 text-xs',
      md: 'h-9 w-9 text-sm',
      lg: 'h-12 w-12 text-base',
    },
  },
  defaultVariants: {
    size: 'md',
  },
})

export type AvatarVariants = VariantProps<typeof avatarVariants>
