/**
 * 学号（候选人身份键）的归一化与校验 —— 与后端 model.ValidateStudentNo 同规则：
 * 纯数字、1–64 位；归一化 = 去除首尾空白（含全角空格）+ 全角数字折半角；
 * 前导零有意义（`00123` ≠ `123`），故一律按字符串处理。
 * 前端校验只做即时反馈，权威判定仍在服务端（唯一性也只有服务端能定）。
 */

const STUDENT_NO_MAX_LEN = 64

/** 归一化并校验学号：合法返回归一化值，否则返回中文原因。 */
export function normalizeStudentNo(raw: string): { value: string } | { error: string } {
  const trimmed = raw.trim()
  if (!trimmed) return { error: '学号不能为空' }
  // 全角数字（U+FF10–U+FF19）→ 半角
  const folded = trimmed.replace(/[\uFF10-\uFF19]/g, (c) =>
    String.fromCharCode(c.charCodeAt(0) - 0xff10 + 0x30),
  )
  if (!/^\d+$/.test(folded)) return { error: '学号只能包含数字' }
  if (folded.length > STUDENT_NO_MAX_LEN) return { error: `学号长度不能超过 ${STUDENT_NO_MAX_LEN} 位` }
  return { value: folded }
}
