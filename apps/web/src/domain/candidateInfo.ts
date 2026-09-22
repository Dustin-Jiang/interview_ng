/**
 * 候选人资料表单（CandidateInfoPayload）的纯函数：空表单与由候选人回填。
 * 「候选人管理」页新增/编辑对话框共用（编辑为全量覆盖，故回填全部字段）。
 */
import type { Candidate, CandidateInfoPayload } from '@/models'

/** 空资料表单（除必填外全部留空）。 */
export function blankCandidateInfo(): CandidateInfoPayload {
  return {
    student_no: '',
    name: '',
    profile: '',
    first_choice: '',
    second_choice: '',
    accept_adjust: false,
    phone: '',
    qq: '',
    email: '',
  }
}

/** 由候选人回填编辑表单（可空字段按空串 / 「不接受调剂」）。 */
export function candidateInfoOf(candidate: Candidate): CandidateInfoPayload {
  return {
    student_no: candidate.student_no,
    name: candidate.name,
    profile: candidate.profile ?? '',
    first_choice: candidate.first_choice ?? '',
    second_choice: candidate.second_choice ?? '',
    accept_adjust: candidate.accept_adjust ?? false,
    phone: candidate.phone ?? '',
    qq: candidate.qq ?? '',
    email: candidate.email ?? '',
  }
}
