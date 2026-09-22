/**
 * useEntityDialog —— 增改对话框（FormDialog）的「新建 / 编辑」状态组合式函数。
 * 收编各管理页逐字重复的四件套：open / target（null = 新建）/ form / saving，
 * 以及 openCreate / openEdit / submit 三个动作。
 * 提交前 `validate` 返回错误文案即中止并提示（form 可变，可在此归一化，如学号）；
 * 提交成功即关闭对话框并按 `success()` 提示，失败留在对话框上（toastError）。
 */
import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { toast } from 'vue-sonner'

import { toastError } from '@/lib/toast'

export interface UseEntityDialog<Entity, Form> {
  /** 对话框开关（FormDialog 的 open）。 */
  readonly open: Ref<boolean>
  /** 编辑目标（null = 新建）。 */
  readonly target: Ref<Entity | null>
  /** 是否处于编辑态（标题 / 提交文案据此切换）。 */
  readonly editing: ComputedRef<boolean>
  /** 表单草稿（对话框内容区绑定）。 */
  readonly form: Ref<Form>
  readonly saving: Ref<boolean>
  /** 打开新建对话框（表单重置为 blank()）。 */
  openCreate: () => void
  /** 打开某个目标的对话框（表单由 toForm(entity) 填充）。 */
  openEdit: (entity: Entity) => void
  /** 校验并提交（成功即关闭对话框）。 */
  submit: () => Promise<void>
}

export interface EntityDialogOptions<Entity, Form> {
  /** 新建时的空表单。 */
  blank: () => Form
  /** 由目标实体填充编辑表单。 */
  toForm: (entity: Entity) => Form
  /** 提交前校验：返回错误文案即中止（form 可变，可在此归一化）。 */
  validate?: (form: Form, entity: Entity | null) => string | null
  /** 落库动作（entity 为 null 即新建）。 */
  action: (form: Form, entity: Entity | null) => Promise<void>
  /** 成功提示文案。 */
  success: (form: Form, entity: Entity | null) => string
}

export function useEntityDialog<Entity, Form>(
  opts: EntityDialogOptions<Entity, Form>,
): UseEntityDialog<Entity, Form> {
  const open = ref(false)
  const target = ref<Entity | null>(null) as Ref<Entity | null>
  const form = ref(opts.blank()) as Ref<Form>
  const saving = ref(false)
  const editing = computed(() => target.value !== null)

  function openCreate(): void {
    target.value = null
    form.value = opts.blank()
    saving.value = false
    open.value = true
  }

  function openEdit(entity: Entity): void {
    target.value = entity
    form.value = opts.toForm(entity)
    saving.value = false
    open.value = true
  }

  async function submit(): Promise<void> {
    if (saving.value) return
    const entity = target.value
    const message = opts.validate?.(form.value, entity)
    if (message) {
      toast.error(message)
      return
    }
    saving.value = true
    try {
      await opts.action(form.value, entity)
      open.value = false
      toast.success(opts.success(form.value, entity))
    } catch (e) {
      toastError(e)
    } finally {
      saving.value = false
    }
  }

  return { open, target, editing, form, saving, openCreate, openEdit, submit }
}
