import { cva, type VariantProps } from 'class-variance-authority'

export { default as Kbd } from './Kbd.vue'

/**
 * 键帽（shadcn Kbd）：快捷键提示的唯一实现——把「按哪个键」以键帽形式
 * 直接放进控件里（按钮内、段式控件项内），取代只藏在 `title`/`aria-label` 里的纯文本提示
 * （原生 tooltip 在触屏上完全看不到）。
 * 非交互表面：`pointer-events-none` 让点击穿透到所在控件，也不参与选中。
 * 两档底色按**所在表面**选，不是按喜好：
 *  - `muted`（默认）：背景 / 描边类表面（页面、卡片、`variant="outline"` 按钮）；
 *  - `surface`：与 `muted` 同色的表面（如段式控件的 `bg-muted` 轨道）——
 *    muted 键帽落在那里会与底色同色而消失，故改用 `bg-background` + 描边定位。
 */
export const kbdVariants = cva(
  'pointer-events-none inline-flex h-5 w-fit min-w-5 select-none items-center justify-center gap-1 rounded-sm px-1 font-sans text-xs font-medium',
  {
    variants: {
      tone: {
        muted: 'bg-muted text-foreground',
        surface: 'border border-border bg-background text-foreground',
      },
    },
    defaultVariants: {
      tone: 'muted',
    },
  },
)

export type KbdVariants = VariantProps<typeof kbdVariants>
