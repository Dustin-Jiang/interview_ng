/**
 * OIDC 映射规则的浏览器侧编译与试算（domain 层：纯函数，无请求、无 UI 状态）。
 * 角色规则与部门规则共用本模块：入参只要求 `{ expression }`，目标 id 字段由调用方自行解读。
 *
 * 与后端 `internal/oidcauth` 的求值语义保持一致：按顺序逐条求值、首个命中生效；
 * 命中的判定为「结果非 null、非 false、非空串、非空数组、非空对象」——数字（含 0）算命中。
 *
 * 本模块是 `@jmespath-community/jmespath` 的第二个引用点，**只允许被设置页的懒加载 chunk 引用**
 * （登录页只引 `domain/oidc.ts`，不带该库）。
 */
import { compile, TreeInterpreter } from '@jmespath-community/jmespath'
import type { JSONValue } from '@jmespath-community/jmespath'

/** 规则的最小形状：两类规则都满足（目标 id 字段名不同，由调用方读）。 */
export interface OidcRuleLike {
  expression: string
}

/** 编译后的规则：`position` 为规则在列表中的原始下标（0 起，展示为第 N 条）。 */
export interface CompiledRule<T extends OidcRuleLike = OidcRuleLike> {
  rule: T
  node: Parameters<typeof TreeInterpreter.search>[0]
  position: number
}

/** 编译规则表达式；返回成功编译的规则与逐条错误（键为规则下标，0 起）。 */
export function compileRules<T extends OidcRuleLike>(rules: readonly T[]): {
  compiled: CompiledRule<T>[]
  errors: Record<number, string>
} {
  const compiled: CompiledRule<T>[] = []
  const errors: Record<number, string> = {}
  rules.forEach((rule, position) => {
    const expression = rule.expression.trim()
    if (!expression) {
      errors[position] = '表达式不能为空'
      return
    }
    try {
      compiled.push({ rule, node: compile(expression), position })
    } catch (e) {
      errors[position] = e instanceof Error ? e.message : String(e)
    }
  })
  return { compiled, errors }
}

/** 解析粘贴的声明 JSON 文本；失败返回错误文案（声明必须是 JSON 对象）。 */
export function parseClaimsJson(text: string): { claims: JSONValue | null; error: string | null } {
  const trimmed = text.trim()
  if (!trimmed) return { claims: null, error: '请输入声明 JSON' }
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch (e) {
    return { claims: null, error: e instanceof Error ? e.message : String(e) }
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { claims: null, error: '声明必须是 JSON 对象' }
  }
  return { claims: parsed as JSONValue, error: null }
}

/** 命中判定（与后端 oidcauth.Hit 同语义）。 */
export function isRuleHit(result: unknown): boolean {
  if (result === null || result === undefined || result === false || result === '') return false
  if (Array.isArray(result)) return result.length > 0
  if (typeof result === 'object') return Object.keys(result as object).length > 0
  return true
}

/** 与后端 match 同语义：首个命中返回其规则与序号（1 起）；无命中 → null。 */
export function matchRule<T extends OidcRuleLike>(
  compiled: readonly CompiledRule<T>[],
  claims: JSONValue,
): { index: number; rule: T } | null {
  for (const item of compiled) {
    if (isRuleHit(TreeInterpreter.search(item.node, claims))) {
      return { index: item.position + 1, rule: item.rule }
    }
  }
  return null
}
