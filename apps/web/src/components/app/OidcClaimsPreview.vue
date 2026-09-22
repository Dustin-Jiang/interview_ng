<!--
  OidcClaimsPreview —— 规则验证：粘贴 ID token 声明 JSON，按当前两类规则试算命中结果。
  求值语义与后端 oidcauth 一致（顺序求值、首个命中生效；空数组/空对象/空串/false 不算命中）。
  未命中的后果两类不同：角色规则未命中 = 拒绝登录；部门规则未命中 = 不改变账号现有部门。
-->
<script setup lang="ts">
import { computed, ref } from 'vue'

import { compileRules, matchRule, parseClaimsJson } from '@/domain/oidcRules'
import type { OidcDeptRulePayload, OidcRulePayload } from '@/models'

import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

const props = defineProps<{
  roleRules: OidcRulePayload[]
  departmentRules: OidcDeptRulePayload[]
  roles: readonly { id: number; label: string }[]
  departments: readonly { id: number; label: string }[]
}>()

/** 归一后的规则：两类规则表达式相同，只是目标字段名不同 → 试算逻辑只认 target_id。 */
interface PreviewRule {
  expression: string
  target_id: number
}

/** 一类规则的试算上下文。 */
interface KindSpec {
  rules: readonly PreviewRule[]
  kindLabel: string
  targets: readonly { id: number; label: string }[]
  unmatched: string
}

const claimsText = ref('{"groups":["interview-interviewers"]}')

const parsed = computed(() => parseClaimsJson(claimsText.value))

const roleSpec = computed<KindSpec>(() => ({
  rules: props.roleRules.map((r) => ({ expression: r.expression, target_id: r.role_id })),
  kindLabel: '角色规则',
  targets: props.roles,
  unmatched: '未命中任何角色规则：该账号将被拒绝登录',
}))
const departmentSpec = computed<KindSpec>(() => ({
  rules: props.departmentRules.map((r) => ({ expression: r.expression, target_id: r.department_id })),
  kindLabel: '部门规则',
  targets: props.departments,
  unmatched: '未命中任何部门规则：不改变账号现有部门',
}))

/** 试算一类规则：声明/表达式错误 → error；命中 → 规则序号 + 目标名；无命中 → 该类规则的后果。 */
function evaluate(spec: KindSpec): { kind: 'error' | 'hit' | 'none'; message: string } {
  if (parsed.value.error) return { kind: 'error', message: parsed.value.error }
  const { compiled, errors } = compileRules(spec.rules)
  const firstError = Object.entries(errors)[0]
  if (firstError) {
    return { kind: 'error', message: `第 ${Number(firstError[0]) + 1} 条${spec.kindLabel}：${firstError[1]}` }
  }
  const hit = matchRule(compiled, parsed.value.claims!)
  if (!hit) return { kind: 'none', message: spec.unmatched }
  const label = spec.targets.find((t) => t.id === hit.rule.target_id)?.label ?? `未知${spec.kindLabel.slice(0, 2)}`
  return { kind: 'hit', message: `命中第 ${hit.index} 条规则 · ${label}` }
}

const roleOutcome = computed(() => evaluate(roleSpec.value))
const departmentOutcome = computed(() => evaluate(departmentSpec.value))

/** 错误样式（声明 JSON 非法或表达式编译失败）与结论样式分开：后者是正常结果。 */
function resultClass(outcome: { kind: 'error' | 'hit' | 'none' }): string {
  if (outcome.kind === 'error') return 'break-all text-destructive'
  return outcome.kind === 'hit' ? 'font-medium' : 'text-muted-foreground'
}
</script>

<template>
  <div class="grid gap-2">
    <Label for="claims-preview">ID token 声明</Label>
    <Textarea
      id="claims-preview"
      v-model="claimsText"
      rows="4"
      class="font-mono text-xs"
      placeholder='{"groups":["interview-interviewers"]}'
    />
  </div>

  <!-- 触屏窄屏：结论里的表达式片段（无空格的拉丁串）靠 min-w-0 + break-all 折行，不撑宽卡片。 -->
  <dl class="grid gap-1 text-sm">
    <div class="flex gap-2">
      <dt class="shrink-0 text-muted-foreground">角色</dt>
      <dd class="min-w-0" :class="resultClass(roleOutcome)">{{ roleOutcome.message }}</dd>
    </div>
    <div class="flex gap-2">
      <dt class="shrink-0 text-muted-foreground">部门</dt>
      <dd class="min-w-0" :class="resultClass(departmentOutcome)">{{ departmentOutcome.message }}</dd>
    </div>
  </dl>
</template>
