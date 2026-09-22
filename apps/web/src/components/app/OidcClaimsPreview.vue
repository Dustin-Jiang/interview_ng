<!--
  OidcClaimsPreview —— 规则验证：粘贴 ID token 声明 JSON，按当前规则试算命中结果。
  求值语义与后端 oidcauth 一致（顺序求值、首个命中生效；空数组/空对象/空串/false 不算命中）。
-->
<script setup lang="ts">
import { computed, ref } from 'vue'

import { compileRules, matchRule, parseClaimsJson } from '@/domain/oidcRules'
import type { OidcRulePayload, Role } from '@/models'

import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

const props = defineProps<{
  rules: OidcRulePayload[]
  roles: readonly Role[]
}>()

const claimsText = ref('{"groups":["interview-interviewers"]}')

const parsed = computed(() => parseClaimsJson(claimsText.value))
const compiled = computed(() => compileRules(props.rules))

/** 试算结果：错误 / 命中（含规则序号与角色）/ 未命中。 */
const outcome = computed(() => {
  if (parsed.value.error) {
    return { kind: 'error' as const, message: parsed.value.error }
  }
  const firstError = Object.entries(compiled.value.errors)[0]
  if (firstError) {
    return { kind: 'error' as const, message: `第 ${Number(firstError[0]) + 1} 条规则：${firstError[1]}` }
  }
  const hit = matchRule(compiled.value.compiled, parsed.value.claims!)
  if (!hit) return { kind: 'none' as const, message: '未命中任何规则' }
  const roleName = props.roles.find((r) => r.id === hit.rule.role_id)?.name ?? '未知角色'
  return { kind: 'hit' as const, message: `命中第 ${hit.index} 条规则 · 角色：${roleName}` }
})
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

  <p
    v-if="outcome.kind === 'error'"
    class="text-sm break-all text-destructive"
  >
    {{ outcome.message }}
  </p>
  <p v-else class="text-sm font-medium">{{ outcome.message }}</p>
</template>
