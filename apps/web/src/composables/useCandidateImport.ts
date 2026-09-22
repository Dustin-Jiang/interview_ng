/**
 * useCandidateImport —— 管理员数据导入的组合式函数（函数式 ViewModel）。
 *
 * 三步状态机：① 选文件 → ② 字段映射 + 实时预览 → ③ 确认与提交；
 * 只允许回退（保留已选文件与已填映射），不可跳步；映射表达式输入 300ms 防抖后重算预览。
 * 解析与映射是 domain 层的纯函数，这里只负责流程、请求与持久化（自动记住最后一次映射表达式）。
 */
import { computed, onScopeDispose, ref, watch, type ComputedRef, type Ref } from 'vue'
import { toast } from 'vue-sonner'

import { ApiError, candidateApi } from '@/api/http'
import {
  CANDIDATE_IMPORT_FIELDS,
  emptyMapping,
  emptyOutcome,
  mapSheet,
  parseWorkbook,
  toImportPayload,
  type CandidateImportFieldKey,
  type CandidateImportMapping,
  type ImportOutcome,
  type ImportSheet,
} from '@/domain/import'
import { toastError } from '@/lib/toast'
import type { CandidateImportReport, CandidateImportRowError } from '@/models'

/** 最后一次映射表达式的本地存储键（前端偏好，不入库）。 */
const MAPPING_KEY = 'interview_ng_import_mapping'
/** 已移除的命名预设键（历史数据，读到即清理）。 */
const LEGACY_PRESET_KEY = 'interview_ng_import_presets'
/** 表达式输入防抖（与列表刷新同一档位）。 */
const DEBOUNCE_MS = 300
/** 步骤总数（① 选文件 ② 映射+预览 ③ 确认提交）。 */
const STEP_COUNT = 3

export interface UseCandidateImport {
  /** 当前步骤（1–3）。 */
  readonly step: Ref<number>
  /** 已解析的源表（未选文件为 null）。 */
  readonly sheet: Ref<ImportSheet | null>
  readonly parsing: Ref<boolean>
  readonly parseError: Ref<string | null>
  /** 输入框即时绑定的映射（防抖后参与预览计算）。 */
  readonly mapping: Ref<CandidateImportMapping>
  /** 当前映射 + 已有学号的映射结果（含统计与行级问题）。 */
  readonly outcome: ComputedRef<ImportOutcome>
  /** 既有学号索引的加载态 / 失败原因（失败即禁止提交，避免「新建/更新」统计失真）。 */
  readonly existingLoading: Ref<boolean>
  readonly existingError: Ref<string | null>
  readonly submitting: Ref<boolean>
  /** 落库报告（提交成功后有值）。 */
  readonly report: Ref<CandidateImportReport | null>
  /** 服务端行级错误（整批被拒时有值）。 */
  readonly rowErrors: Ref<CandidateImportRowError[]>
  /** 是否可提交（有文件、映射与行全部合法、既有学号索引就绪）。 */
  readonly canSubmit: ComputedRef<boolean>
  pickFile: (file: File) => Promise<void>
  clearFile: () => void
  setExpression: (key: CandidateImportFieldKey, value: string) => void
  goStep: (step: number) => void
  reloadExisting: () => Promise<void>
  submit: () => Promise<void>
  /** 回到第一步并清空全部状态（提交完成后「继续导入下一批」）。 */
  reset: () => void
}

/** 读取上次保存的映射表达式（形状不符或存储不可用时回退到默认表达式）。 */
function readStoredMapping(): CandidateImportMapping {
  const fallback = emptyMapping()
  try {
    localStorage.removeItem(LEGACY_PRESET_KEY)
    const raw = localStorage.getItem(MAPPING_KEY)
    if (!raw) return fallback
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) return fallback
    const stored = parsed as Record<string, unknown>
    for (const field of CANDIDATE_IMPORT_FIELDS) {
      const value = stored[field.key]
      if (typeof value === 'string') fallback[field.key] = value
    }
  } catch {
    // 存储不可用（隐私模式 / 数据损坏）：按默认表达式处理，不阻断导入。
  }
  return fallback
}

export function useCandidateImport(): UseCandidateImport {
  const step = ref(1)
  const sheet = ref<ImportSheet | null>(null)
  const parsing = ref(false)
  const parseError = ref<string | null>(null)
  // 初始值即「最后一次使用过的表达式」（localStorage），免去每次重新填写。
  const mapping = ref<CandidateImportMapping>(readStoredMapping())
  /** 防抖后的映射（预览只认它，避免每次按键都重算全表），同时持久化为「最后状态」。 */
  const appliedMapping = ref<CandidateImportMapping>({ ...mapping.value })
  const existing = ref<ReadonlySet<string>>(new Set())
  const existingLoading = ref(false)
  const existingError = ref<string | null>(null)
  const submitting = ref(false)
  const report = ref<CandidateImportReport | null>(null)
  const rowErrors = ref<CandidateImportRowError[]>([])

  let timer: ReturnType<typeof setTimeout> | null = null
  function cancelPending(): void {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }
  onScopeDispose(cancelPending)

  watch(mapping, () => {
    cancelPending()
    timer = setTimeout(() => {
      timer = null
      appliedMapping.value = { ...mapping.value }
      persistMapping(appliedMapping.value)
    }, DEBOUNCE_MS)
  }, { deep: true })

  const outcome = computed<ImportOutcome>(() =>
    sheet.value ? mapSheet(sheet.value, appliedMapping.value, existing.value) : emptyOutcome(),
  )

  const canSubmit = computed(() => {
    if (!sheet.value || submitting.value || existingLoading.value || existingError.value) return false
    if (Object.keys(outcome.value.expressionErrors).length > 0) return false
    const rows = outcome.value.mapped
    return rows.length > 0 && rows.every((r) => r.status !== 'error')
  })

  /** 拉取全部既有学号（listAll 逐页拉全）：预览据此区分「新建 / 更新」。 */
  async function reloadExisting(): Promise<void> {
    existingLoading.value = true
    existingError.value = null
    try {
      const { items } = await candidateApi.listAll()
      existing.value = new Set(items.map((c) => c.student_no))
    } catch (e) {
      existingError.value = e instanceof Error ? e.message : String(e)
    } finally {
      existingLoading.value = false
    }
  }

  async function pickFile(file: File): Promise<void> {
    cancelPending()
    parsing.value = true
    parseError.value = null
    report.value = null
    rowErrors.value = []
    try {
      const parsed = parseWorkbook(await file.arrayBuffer(), file.name)
      sheet.value = parsed
      appliedMapping.value = { ...mapping.value }
      await reloadExisting()
      step.value = 2
    } catch (e) {
      sheet.value = null
      existing.value = new Set()
      parseError.value = e instanceof Error ? e.message : String(e)
    } finally {
      parsing.value = false
    }
  }

  function clearFile(): void {
    sheet.value = null
    parseError.value = null
    report.value = null
    rowErrors.value = []
    existing.value = new Set()
    step.value = 1
  }

  function setExpression(key: CandidateImportFieldKey, value: string): void {
    mapping.value = { ...mapping.value, [key]: value }
  }

  /** 步骤导航：可回退到任意前序步骤；前进仅限「下一步」且目标步骤已解锁。 */
  function goStep(next: number): void {
    if (next < 1 || next > STEP_COUNT) return
    if (next > step.value + 1) return
    if (next >= 2 && !sheet.value) return
    if (next === STEP_COUNT && !canSubmit.value) return
    step.value = next
  }

  /** 持久化当前表达式（静默降级：存储不可用时仅本次会话有效）。 */
  function persistMapping(value: CandidateImportMapping): void {
    try {
      localStorage.setItem(MAPPING_KEY, JSON.stringify(value))
    } catch {
      // 隐私模式 / 配额不足：不阻断导入主流程。
    }
  }

  async function submit(): Promise<void> {
    if (!canSubmit.value) return
    submitting.value = true
    report.value = null
    rowErrors.value = []
    try {
      const result = await candidateApi.import(toImportPayload(outcome.value.mapped))
      report.value = result
      toast.success(`导入完成：新建 ${result.created} 人，更新 ${result.updated} 人`)
    } catch (e) {
      // 整批被拒：取出服务端的行级报告（行号 + 原因），界面对照到源表行。
      const rows = e instanceof ApiError ? (e.data as { rows?: CandidateImportRowError[] } | undefined)?.rows : undefined
      if (rows?.length) {
        rowErrors.value = rows
        toast.error('导入被服务端拒绝，请按行级报告修正后重传')
      } else {
        toastError(e)
      }
    } finally {
      submitting.value = false
    }
  }

  function reset(): void {
    cancelPending()
    step.value = 1
    sheet.value = null
    parsing.value = false
    parseError.value = null
    existing.value = new Set()
    existingError.value = null
    report.value = null
    rowErrors.value = []
  }

  return {
    step,
    sheet,
    parsing,
    parseError,
    mapping,
    outcome,
    existingLoading,
    existingError,
    submitting,
    report,
    rowErrors,
    canSubmit,
    pickFile,
    clearFile,
    setExpression,
    goStep,
    reloadExisting,
    submit,
    reset,
  }
}
