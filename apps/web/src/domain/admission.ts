/**
 * 录取预览领域 —— 纯函数层。
 * 由「候选人 × 部门 × 录取决定」三份原始数据推导预览矩阵与每位候选人的汇总结论。
 * 无关 Vue，无副作用；输入不被修改。
 */
import type { AdmissionStatus } from '@/models'

/** 候选人的录取汇总结论。 */
export type AdmissionOutcome =
  | { kind: 'admitted'; departmentName: string } // 唯一部门录取，且其余部门全部放弃
  | { kind: 'none' } // 所有部门均放弃，无人录取
  | { kind: 'conflict' } // 多个部门同时录取
  | { kind: 'pending' } // 仍有部门待定，结论未定

/** 预览表的一行：候选人 + 各部门决定（与部门列表顺序一致）+ 汇总结论。 */
export interface AdmissionPreviewRow {
  candidateId: number
  candidateName: string
  statuses: AdmissionStatus[]
  outcome: AdmissionOutcome
}

/** 录取决定的最小形状（与后端 CandidateAdmission JSON 字段一致）。 */
export interface AdmissionRecord {
  candidate_id: number
  department_id: number
  status: AdmissionStatus
}

/**
 * 由某候选人的各部门决定推导汇总结论。
 * statuses 与 departmentNames 按相同部门顺序一一对应（未记录的决定由调用方补为 pending）。
 * 唯一录取 + 其余全部放弃才落定为「录取到该部门」；多家录取视为冲突；存在待定则结论未定。
 */
export function admissionOutcomeOf(
  statuses: readonly AdmissionStatus[],
  departmentNames: readonly string[],
): AdmissionOutcome {
  let admittedCount = 0
  let pendingCount = 0
  let admittedIndex = -1
  for (let i = 0; i < statuses.length; i++) {
    if (statuses[i] === 'admitted') {
      admittedCount += 1
      admittedIndex = i
    } else if (statuses[i] === 'pending') {
      pendingCount += 1
    }
  }
  if (admittedCount > 1) return { kind: 'conflict' }
  if (admittedCount === 1) {
    if (pendingCount > 0) return { kind: 'pending' }
    return { kind: 'admitted', departmentName: departmentNames[admittedIndex] ?? '' }
  }
  return pendingCount > 0 ? { kind: 'pending' } : { kind: 'none' }
}

/**
 * 构建录取预览行（保持候选人与部门的给定顺序）。
 * admissions 中缺失的 (候选人, 部门) 组合视为待定。
 */
export function buildAdmissionPreview(
  candidates: readonly { id: number; name: string }[],
  departments: readonly { id: number; name: string }[],
  admissions: readonly AdmissionRecord[],
): AdmissionPreviewRow[] {
  const byKey = new Map<string, AdmissionStatus>()
  for (const a of admissions) {
    byKey.set(`${a.candidate_id}:${a.department_id}`, a.status)
  }
  const departmentNames = departments.map((d) => d.name)
  return candidates.map((c) => {
    const statuses = departments.map((d) => byKey.get(`${c.id}:${d.id}`) ?? 'pending')
    return {
      candidateId: c.id,
      candidateName: c.name,
      statuses,
      outcome: admissionOutcomeOf(statuses, departmentNames),
    }
  })
}
