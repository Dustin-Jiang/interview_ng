/**
 * 录取预览领域 —— 纯函数层。
 * 由「候选人 × 部门 × 录取决定」三份原始数据推导预览矩阵与每位候选人的汇总结论。
 * 无关 Vue，无副作用；输入不被修改。
 */
import type { AdmissionStatus, SystemPhase } from '@/models'

/**
 * 该系统阶段是否允许各部门记录录取决定（「录取 / 放弃 / 待定」）。
 * **面试阶段起就可以表态**——面试现场就能给出结论，不必等到录取阶段；结算阶段不允许：
 * 那时竞拍已自动结算、录取档已批量同步，手动改没有意义（与「出价在结算阶段只读」同一口径）。
 * 服务端**不设阶段门**（`UpsertCandidateAdmission` 只校验候选人 / 部门 / 状态），
 * 阶段只决定界面是否给出入口——故这条策略是唯一判据，界面各处以它为准，别各自判断。
 */
export function isAdmissionRecordingPhase(phase: SystemPhase | null): boolean {
  return phase === 'interview' || phase === 'admission' || phase === 'leftover'
}

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
 * statuses 与 departmentNames 按相同部门顺序一一对应（**未记录的决定由调用方补为弃权**）。
 * 唯一录取 + 其余全部放弃才落定为「录取到该部门」；多家录取视为冲突；显式「待定」则结论未定。
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
 * admissions 中缺失的 (候选人, 部门) 组合 = 该部门未表态 → **弃权**：不表态不影响录取判定；
 * 只有显式记录的「待定」才表示结论未定。
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
    const statuses = departments.map((d) => byKey.get(`${c.id}:${d.id}`) ?? 'withdrawn')
    return {
      candidateId: c.id,
      candidateName: c.name,
      statuses,
      outcome: admissionOutcomeOf(statuses, departmentNames),
    }
  })
}
