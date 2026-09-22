/**
 * 候选人导入的浏览器侧解析与映射（domain 层：纯函数，无请求、无 UI 状态）。
 *
 * 分工与依据：
 *  - 服务端不解析表格：只收「已映射好的行」，校验与落库由后端单事务全或无负责；
 *  - 行对象 = 首行表头为 key，值做类型推断（数字 / 布尔 / 日期 → ISO 字符串 / 文本），
 *    空单元格为 null，全空行跳过；
 *  - 学号取单元格原始值（数值单元格的格式化文本可能带千分位等格式，会破坏纯数字校验）；
 *  - 映射结果按 JSON 转义集解释字面量转义（`\n` → 换行等），使真实换行与字面量换行都能正确显示；
 *  - JMESPath 中非 ASCII 起首的标识符必须加引号（`"姓名"`），各实现一致强制；
 *    中文列名因此以 `"列名"` 形式出现在表达式里，界面直接提供可复制的列名清单。
 */
import { compile, TreeInterpreter } from '@jmespath-community/jmespath'
import type { JSONValue } from '@jmespath-community/jmespath'
import * as XLSX from 'xlsx/dist/xlsx.mini.min.js'

import { normalizeStudentNo } from '@/domain/studentNo'

/** 单个文件的解析上限：行数与字节数（体积在读取前先量，行数在解析后校验）。 */
export const IMPORT_MAX_ROWS = 2000
export const IMPORT_MAX_BYTES = 5 * 1024 * 1024

/** 源表的一行：行号（表头为第 1 行，数据自第 2 行起）+ 行对象。 */
export interface ImportRow {
  line: number
  values: Record<string, unknown>
}

/** 解析后的源表（单工作表）。 */
export interface ImportSheet {
  fileName: string
  sheetName: string
  headers: string[]
  rows: ImportRow[]
}

/** 目标字段定义（映射模型：每个目标字段一条 JMESPath 表达式）。 */
export const CANDIDATE_IMPORT_FIELDS = [
  { key: 'student_no', label: '学号', required: true },
  { key: 'name', label: '姓名', required: true },
  { key: 'profile', label: '个人简介', required: false },
] as const

export type CandidateImportFieldKey = (typeof CANDIDATE_IMPORT_FIELDS)[number]['key']

/** 映射：目标字段 → JMESPath 表达式。 */
export type CandidateImportMapping = Record<CandidateImportFieldKey, string>

/** 命名预设（一组表达式；持久化由调用方负责）。 */
export interface ImportPreset {
  name: string
  mapping: CandidateImportMapping
}

/** 映射后的单行（与源表数据行一一对应、同序，便于三段对照）。 */
export interface ImportMappedRow {
  /** 源文件行号。 */
  line: number
  studentNo: string
  name: string
  profile: string
  /** 行级错误（非空即该行不合法；提交为全或无，任一行不合法会被服务端整批拒绝）。 */
  errors: string[]
  /** 行结果：新建 / 命中既有学号（或批内前行）覆盖 / 不合法。 */
  status: 'create' | 'update' | 'error'
  /** 本批内被本行覆盖的行号（批内同学号：后行覆盖前行）。 */
  overridesLine?: number
}

/** 映射结果：表达式级错误 + 行结果 + 统计（统计口径与后端报告一致）。 */
export interface ImportOutcome {
  expressionErrors: Partial<Record<CandidateImportFieldKey, string>>
  mapped: ImportMappedRow[]
  stats: { total: number; create: number; update: number; failed: number }
}

/** 空映射（进入导入页时的初始表达式：按列名必须加引号的规则给出可复制样例）。 */
export function emptyMapping(): CandidateImportMapping {
  return { student_no: '', name: '"姓名"', profile: '' }
}

/** 空映射结果（尚未选文件 / 表达式未通过编译时使用）。 */
export function emptyOutcome(): ImportOutcome {
  return { expressionErrors: {}, mapped: [], stats: { total: 0, create: 0, update: 0, failed: 0 } }
}

/**
 * 编译后的 JMESPath 表达式节点：依赖未导出该类型，这里按求值接口的参数签名取名，
 * 供「编译一次、多行复用」的映射流程与调用方共用。
 */
export type CompiledExpression = Parameters<typeof TreeInterpreter.search>[0]

/** 单元格取值：空 → null；日期 → ISO 字符串（UTC 解释）；其余保留原始类型。 */
function cellValue(cell: unknown): unknown {
  if (cell === null || cell === undefined || cell === '') return null
  if (cell instanceof Date) return cell.toISOString()
  return cell
}

/** 单元格文本（表头用）：空 → ''，日期 → ISO 字符串。 */
function cellText(cell: unknown): string {
  const v = cellValue(cell)
  return v === null ? '' : String(v)
}

/**
 * 解析 .xlsx 工作簿（仅单工作表）。任一前置条件不满足即抛错，文案直指可执行的修正动作。
 * @param data 文件字节
 * @param fileName 原始文件名（报告与界面展示用）
 */
export function parseWorkbook(data: ArrayBuffer, fileName: string): ImportSheet {
  // 只接受 .xlsx：SheetJS 对 .csv 等文本格式会按二进制猜测编码（中文表头会变成乱码），
  // 与其静默读出错列名，不如在上传环节直接拒绝。
  if (!/\.xlsx$/i.test(fileName)) {
    throw new Error('只支持 .xlsx 工作簿')
  }
  if (data.byteLength > IMPORT_MAX_BYTES) {
    throw new Error(`文件超过 ${IMPORT_MAX_BYTES / 1024 / 1024}MB，请拆分后重传`)
  }
  let book: XLSX.WorkBook
  try {
    // cellDates: 日期单元格解析为 Date（UTC），在行对象里统一转 ISO 字符串。
    book = XLSX.read(new Uint8Array(data), { type: 'array', cellDates: true })
  } catch {
    throw new Error('无法解析该文件，请确认是 .xlsx 工作簿')
  }
  const sheetName = book.SheetNames[0]
  if (!sheetName) throw new Error('工作簿中没有任何工作表')

  const matrix = XLSX.utils.sheet_to_json<unknown[]>(book.Sheets[sheetName], {
    header: 1,
    raw: true,
    defval: null,
  })
  const headerRow = (matrix[0] ?? []).map((c) => cellText(c).trim())
  if (headerRow.length === 0 || headerRow.some((h) => h === '')) {
    throw new Error('第 1 行存在空表头，请补全列名后重传')
  }
  const duplicated = headerRow.find((h, i) => headerRow.indexOf(h) !== i)
  if (duplicated) throw new Error(`第 1 行表头重复：${duplicated}`)

  const rows: ImportRow[] = []
  // 不传 blankrows：数组模式下空行保留在矩阵里，行号才能与文件实际行号一一对应。
  for (let i = 1; i < matrix.length; i++) {
    const cells = matrix[i] ?? []
    const values: Record<string, unknown> = {}
    let blank = true
    headerRow.forEach((header, column) => {
      const value = cellValue(cells[column])
      values[header] = value
      if (value !== null) blank = false
    })
    if (!blank) rows.push({ line: i + 1, values })
  }
  if (rows.length === 0) throw new Error('工作表里没有数据行')
  if (rows.length > IMPORT_MAX_ROWS) {
    throw new Error(`单次最多导入 ${IMPORT_MAX_ROWS} 行（当前 ${rows.length} 行）`)
  }
  return { fileName, sheetName, headers: headerRow, rows }
}

/**
 * 解释映射结果中的转义序列。
 * 映射结果里的换行有两种来源：单元格内**真实的换行符**（Alt+Enter），以及**字面量转义**
 * ——JMESPath 的原始字符串字面量（`'...'`）按规范不处理转义，单元格里也可能存的就是 `\n` 两个字符。
 * 这里统一按 **JSON 字符串转义集** 解释：`\" \\ \/ \b \f \n \r \t \uXXXX`。
 * 未知/非法转义（如正则里的 `\d`）原样保留；注意 `C:\temp` 这类文本中的 `\t` 会被解释为制表符，
 * 需要保留反斜杠时写 `\\`。
 */
export function interpretEscapes(text: string): string {
  if (!text.includes('\\')) return text
  return text.replace(/\\u[0-9a-fA-F]{4}|\\./gs, (match) => {
    const seq = match.slice(1)
    switch (seq[0]) {
      case '"':
        return '"'
      case '\\':
        return '\\'
      case '/':
        return '/'
      case 'b':
        return '\b'
      case 'f':
        return '\f'
      case 'n':
        return '\n'
      case 'r':
        return '\r'
      case 't':
        return '\t'
      case 'u': {
        const code = Number.parseInt(seq.slice(1), 16)
        return Number.isNaN(code) ? match : String.fromCharCode(code)
      }
      default:
        return match
    }
  })
}

/** 值转文本：null/undefined → ''；对象/数组 → JSON（不静默丢信息）；其余 String()。 */
function toText(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function expressionMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/**
 * 按映射把源表各行求值成候选人字段。表达式先整体编译（编译失败即返回表达式级错误，
 * 不产出行结果），再逐行求值 —— 编译一次、多行复用。
 * @param existingNos 库中已有学号（用于区分「新建 / 更新」的预览统计）
 */
export function mapSheet(
  sheet: ImportSheet,
  mapping: CandidateImportMapping,
  existingNos: ReadonlySet<string>,
): ImportOutcome {
  const expressionErrors: Partial<Record<CandidateImportFieldKey, string>> = {}
  const compiled: Partial<Record<CandidateImportFieldKey, CompiledExpression>> = {}
  for (const field of CANDIDATE_IMPORT_FIELDS) {
    const expression = (mapping[field.key] ?? '').trim()
    if (!expression) {
      if (field.required) expressionErrors[field.key] = '必填字段缺少表达式'
      continue
    }
    try {
      compiled[field.key] = compile(expression)
    } catch (e) {
      expressionErrors[field.key] = expressionMessage(e)
    }
  }
  if (Object.keys(expressionErrors).length > 0) return { ...emptyOutcome(), expressionErrors }

  const evaluate = (key: CandidateImportFieldKey, values: Record<string, unknown>): string => {
    const node = compiled[key]
    if (!node) return ''
    try {
      return toText(TreeInterpreter.search(node, values as JSONValue))
    } catch {
      // 运行期求值异常（表达式本身已编译通过）按空值处理，由必填校验兜底。
      return ''
    }
  }

  const seen = new Map<string, number>()
  const mapped: ImportMappedRow[] = sheet.rows.map((row) => {
    const out: ImportMappedRow = {
      line: row.line,
      studentNo: '',
      name: '',
      profile: '',
      errors: [],
      status: 'create',
    }
    const no = normalizeStudentNo(interpretEscapes(evaluate('student_no', row.values)))
    if ('error' in no) out.errors.push(no.error)
    else out.studentNo = no.value

    out.name = interpretEscapes(evaluate('name', row.values)).trim()
    if (!out.name) out.errors.push('姓名不能为空')
    out.profile = interpretEscapes(evaluate('profile', row.values))

    if (out.errors.length > 0) {
      out.status = 'error'
      return out
    }
    const firstLine = seen.get(out.studentNo)
    if (firstLine !== undefined) {
      // 批内同学号：后行覆盖前行（服务端同一口径）
      out.status = 'update'
      out.overridesLine = firstLine
    } else {
      seen.set(out.studentNo, row.line)
      out.status = existingNos.has(out.studentNo) ? 'update' : 'create'
    }
    return out
  })

  return {
    expressionErrors,
    mapped,
    stats: {
      total: mapped.length,
      create: mapped.filter((r) => r.status === 'create').length,
      update: mapped.filter((r) => r.status === 'update').length,
      failed: mapped.filter((r) => r.status === 'error').length,
    },
  }
}

/** 映射结果 → 提交载荷（与 POST /candidates/imports 契约一致）。 */
export function toImportPayload(
  mapped: readonly ImportMappedRow[],
): { student_no: string; name: string; profile: string }[] {
  return mapped.map((r) => ({ student_no: r.studentNo, name: r.name, profile: r.profile }))
}
